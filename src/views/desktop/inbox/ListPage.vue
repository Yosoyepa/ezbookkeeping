<template>
    <main-page-layout no-navbar>
        <template #content>
            <v-row class="match-height">
                <v-col cols="12">
                    <v-card>
                        <template #title>
                            <div class="title-and-toolbar d-flex align-center">
                                <span>{{ tt('Inbox') }}</span>
                                <v-btn class="ms-3" color="default" variant="outlined"
                                       :disabled="loading || uploading" :loading="uploading"
                                       @click="openStatementPicker">{{ tt('Upload Statement') }}</v-btn>
                                <v-spacer/>
                                <v-btn density="compact" color="default" variant="text" class="ms-2"
                                       :aria-label="tt('Refresh')" :icon="true"
                                       :disabled="loading || uploading" :loading="loading"
                                       @click="reload">
                                    <template #loader>
                                        <v-progress-circular indeterminate size="20"/>
                                    </template>
                                    <v-icon :icon="mdiRefresh" size="24" />
                                    <v-tooltip activator="parent">{{ tt('Refresh') }}</v-tooltip>
                                </v-btn>
                            </div>
                        </template>

                        <v-tabs :model-value="status" color="primary" @update:model-value="onStatusChange">
                            <v-tab :value="InboxItemStatus.Pending">{{ tt('Pending') }}</v-tab>
                            <v-tab :value="InboxItemStatus.Processed">{{ tt('Processed') }}</v-tab>
                            <v-tab :value="InboxItemStatus.Discarded">{{ tt('Discarded') }}</v-tab>
                        </v-tabs>
                        <v-divider/>

                        <v-table class="inbox-table table-striped" :hover="!loading">
                            <thead>
                            <tr>
                                <th>{{ tt('Type') }}</th>
                                <th>{{ tt('Name') }}</th>
                                <th>{{ tt('Date') }}</th>
                                <th>{{ tt('Note') }}</th>
                                <th>{{ tt('Actions') }}</th>
                            </tr>
                            </thead>

                            <tbody v-if="loading && !items.length">
                            <tr :key="itemIdx" v-for="itemIdx in [ 1, 2, 3 ]">
                                <td colspan="5" class="px-0">
                                    <v-skeleton-loader type="text" :loading="true"></v-skeleton-loader>
                                </td>
                            </tr>
                            </tbody>

                            <tbody v-if="!loading && !items.length">
                            <tr>
                                <td colspan="5">{{ status === InboxItemStatus.Pending ? tt('Process your receipts from your CLI with /recibos') : tt('No items') }}</td>
                            </tr>
                            </tbody>

                            <tbody v-if="items.length">
                            <tr :key="item.id" v-for="item in items">
                                <td>
                                    <v-icon :icon="item.kind === InboxItemKind.Receipt ? mdiCameraOutline : mdiFileDocumentOutline"/>
                                </td>
                                <td>
                                    <div class="d-flex align-center">
                                        <img class="inbox-item-thumbnail me-2" alt="" :src="inboxStore.getItemPictureUrl(item)"
                                             v-if="item.kind === InboxItemKind.Receipt && item.pictureUrl" />
                                        <span class="inbox-item-name">{{ getItemName(item) }}</span>
                                    </div>
                                </td>
                                <td>{{ formatCreatedTime(item) }}</td>
                                <td>
                                    <v-btn class="inbox-item-transactions px-2" color="primary" density="comfortable" variant="text"
                                           @click="showTransactions(item)"
                                           v-if="item.resultTransactionIds.length">
                                        {{ tt('Transactions') }} ({{ item.resultTransactionIds.length }})
                                    </v-btn>
                                    <div class="inbox-item-note" v-if="getItemNote(item)">{{ getItemNote(item) }}</div>
                                </td>
                                <td>
                                    <div class="d-flex align-center">
                                        <v-btn class="px-2" color="default" density="comfortable" variant="text"
                                               :prepend-icon="mdiArchiveOutline"
                                               :disabled="loading || updating"
                                               @click="discard(item)"
                                               v-if="item.status === InboxItemStatus.Pending">{{ tt('Discard') }}</v-btn>
                                        <v-btn class="px-2" color="default" density="comfortable" variant="text"
                                               :prepend-icon="mdiBackupRestore"
                                               :disabled="loading || updating"
                                               @click="reopen(item)"
                                               v-if="item.status !== InboxItemStatus.Pending">{{ tt('Reopen') }}</v-btn>
                                        <v-btn class="px-2" color="default" density="comfortable" variant="text"
                                               :prepend-icon="mdiDeleteOutline"
                                               :disabled="loading || updating"
                                               @click="remove(item)"
                                               v-if="item.status !== InboxItemStatus.Processed">{{ tt('Delete') }}</v-btn>
                                    </div>
                                </td>
                            </tr>
                            </tbody>
                        </v-table>

                        <div class="d-flex justify-center pa-3" v-if="!loading && items.length < totalCount">
                            <v-btn color="default" variant="outlined" :loading="loadingMore" :disabled="loadingMore" @click="loadMore">{{ tt('Load More') }}</v-btn>
                        </div>
                    </v-card>
                </v-col>
            </v-row>

            <input ref="statementInput" type="file" style="display: none" :accept="INBOX_STATEMENT_FILE_ACCEPT" @change="onStatementSelected($event)" />
            <v-dialog width="480" scrollable v-model="showTransactionsDialog">
                <v-card>
                    <v-card-title>{{ tt('Transactions') }} ({{ selectedTransactionIds.length }})</v-card-title>
                    <v-card-text class="pa-0">
                        <v-list>
                            <v-list-item :key="transactionId" v-for="(transactionId, index) in selectedTransactionIds"
                                         :title="tt('Transaction') + ' ' + (index + 1)" :subtitle="transactionId"
                                         @click="openTransaction(transactionId)" />
                        </v-list>
                    </v-card-text>
                    <v-card-actions>
                        <v-spacer/>
                        <v-btn color="default" variant="text" @click="showTransactionsDialog = false">{{ tt('Close') }}</v-btn>
                    </v-card-actions>
                </v-card>
            </v-dialog>

            <edit-dialog ref="editDialog" :type="TransactionEditPageType.Transaction" />
            <confirm-dialog ref="confirmDialog"/>
            <snack-bar ref="snackbar" />
        </template>
    </main-page-layout>
</template>

<script setup lang="ts">
import ConfirmDialog from '@/components/desktop/ConfirmDialog.vue';
import SnackBar from '@/components/desktop/SnackBar.vue';
import EditDialog from '@/views/desktop/transactions/list/dialogs/EditDialog.vue';

import { ref, useTemplateRef } from 'vue';

import { useI18n } from '@/locales/helpers.ts';

import { useInboxStore } from '@/stores/inbox.ts';

import { TransactionEditPageType } from '@/views/base/transactions/TransactionEditPageBase.ts';

import {
    InboxItem,
    InboxItemKind,
    InboxItemStatus,
    INBOX_PAGE_SIZE,
    INBOX_STATEMENT_FILE_ACCEPT
} from '@/models/inbox.ts';

import { parseDateTimeFromUnixTime } from '@/lib/datetime.ts';

import {
    mdiRefresh,
    mdiCameraOutline,
    mdiFileDocumentOutline,
    mdiArchiveOutline,
    mdiBackupRestore,
    mdiDeleteOutline
} from '@mdi/js';

type ConfirmDialogType = InstanceType<typeof ConfirmDialog>;
type SnackBarType = InstanceType<typeof SnackBar>;
type EditDialogType = InstanceType<typeof EditDialog>;

const { tt, formatDateTimeToShortDateTime } = useI18n();

const inboxStore = useInboxStore();

const confirmDialog = useTemplateRef<ConfirmDialogType>('confirmDialog');
const snackbar = useTemplateRef<SnackBarType>('snackbar');
const editDialog = useTemplateRef<EditDialogType>('editDialog');
const statementInput = useTemplateRef<HTMLInputElement>('statementInput');

const showTransactionsDialog = ref<boolean>(false);
const selectedTransactionIds = ref<string[]>([]);

const status = ref<InboxItemStatus>(InboxItemStatus.Pending);
const items = ref<InboxItem[]>([]);
const totalCount = ref<number>(0);
const currentPage = ref<number>(1);
const loading = ref<boolean>(true);
const loadingMore = ref<boolean>(false);
const updating = ref<boolean>(false);
const uploading = ref<boolean>(false);

// Loads one page of the current status. A response for a status that is no longer selected is ignored.
function fetchPage(pageNumber: number): Promise<void> {
    const requestedStatus = status.value;

    return inboxStore.loadItems({
        status: requestedStatus,
        count: INBOX_PAGE_SIZE,
        page: pageNumber
    }).then(response => {
        if (requestedStatus !== status.value) {
            return;
        }

        items.value = pageNumber > 1 ? items.value.concat(response.items) : response.items;
        totalCount.value = response.totalCount;
        currentPage.value = pageNumber;
    });
}

function init(): void {
    loading.value = true;

    fetchPage(1).then(() => {
        loading.value = false;
    }).catch(error => {
        loading.value = false;

        if (!error.processed) {
            snackbar.value?.showError(error);
        }
    });
}

function reload(): void {
    loading.value = true;

    fetchPage(1).then(() => {
        loading.value = false;
    }).catch(error => {
        loading.value = false;

        if (!error.processed) {
            snackbar.value?.showError(error);
        }
    });
}

function onStatusChange(value: unknown): void {
    const newStatus = Number(value) as InboxItemStatus;

    if (!newStatus || status.value === newStatus) {
        return;
    }

    status.value = newStatus;
    items.value = [];
    totalCount.value = 0;
    reload();
}

function loadMore(): void {
    if (loadingMore.value || items.value.length >= totalCount.value) {
        return;
    }

    loadingMore.value = true;

    fetchPage(currentPage.value + 1).then(() => {
        loadingMore.value = false;
    }).catch(error => {
        loadingMore.value = false;

        if (!error.processed) {
            snackbar.value?.showError(error);
        }
    });
}

function openStatementPicker(): void {
    if (uploading.value) {
        return;
    }

    statementInput.value?.click();
}

function onStatementSelected(event: Event): void {
    const el = event.target as HTMLInputElement;

    if (!el.files || !el.files.length || !el.files[0]) {
        return;
    }

    const file = el.files[0];

    el.value = '';

    uploading.value = true;

    inboxStore.uploadItem({
        file: file,
        kind: InboxItemKind.Statement
    }).then(() => {
        uploading.value = false;

        snackbar.value?.showMessage('Statement uploaded');

        // the new statement is pending, so show the pending list
        if (status.value !== InboxItemStatus.Pending) {
            status.value = InboxItemStatus.Pending;
            items.value = [];
            totalCount.value = 0;
        }

        reload();
    }).catch(error => {
        uploading.value = false;

        if (!error.processed) {
            snackbar.value?.showError(error);
        }
    });
}

function getItemName(item: InboxItem): string {
    if (item.kind === InboxItemKind.Statement) {
        return item.originalFileName || tt('Statement');
    }

    return tt('Receipt');
}

function formatCreatedTime(item: InboxItem): string {
    return formatDateTimeToShortDateTime(parseDateTimeFromUnixTime(item.createdTime));
}

// The text under the transactions button: the reason of a discarded item, the failed rows of a partially processed item or the note of the upload.
// Processed items only show their transactions.
function getItemNote(item: InboxItem): string {
    if (item.status === InboxItemStatus.Discarded) {
        return item.processedNote || '';
    }

    if (item.status === InboxItemStatus.Pending) {
        return item.processedNote || item.note || '';
    }

    return '';
}

// The transactions of an item are listed from the ids saved in the item (not by searching the comments, which the user can edit and
// which carry the label of the statement instead of the id of the item)
function showTransactions(item: InboxItem): void {
    selectedTransactionIds.value = item.resultTransactionIds;
    showTransactionsDialog.value = true;
}

function openTransaction(transactionId: string): void {
    editDialog.value?.open({
        id: transactionId
    }).catch(error => {
        if (error) {
            snackbar.value?.showError(error);
        }
    });
}

function runItemAction(action: Promise<boolean>): void {
    updating.value = true;

    action.then(() => {
        updating.value = false;
        reload();
    }).catch(error => {
        updating.value = false;

        if (!error.processed) {
            snackbar.value?.showError(error);
        }
    });
}

function discard(item: InboxItem): void {
    runItemAction(inboxStore.discardItem({ item: item }));
}

function reopen(item: InboxItem): void {
    runItemAction(inboxStore.reopenItem({ item: item }));
}

function remove(item: InboxItem): void {
    confirmDialog.value?.open('Are you sure you want to delete this item?').then(() => {
        runItemAction(inboxStore.deleteItem({ item: item }));
    });
}

init();
</script>

<style>
.inbox-item-thumbnail {
    flex: none;
    width: 40px;
    height: 40px;
    object-fit: cover;
    border-radius: 6px;
}

.inbox-item-name {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
}
</style>
