<template>
    <f7-page ptr @ptr:refresh="reload" @page:afterin="onPageAfterIn">
        <f7-navbar>
            <f7-nav-left :class="{ 'disabled': loading }" :back-link="tt('Back')"></f7-nav-left>
            <f7-nav-title :title="tt('Inbox')"></f7-nav-title>
        </f7-navbar>

        <f7-block class="margin-vertical-half">
            <f7-segmented strong>
                <f7-button :active="status === InboxItemStatus.Pending" @click="changeStatus(InboxItemStatus.Pending)">{{ tt('Pending') }}</f7-button>
                <f7-button :active="status === InboxItemStatus.Processed" @click="changeStatus(InboxItemStatus.Processed)">{{ tt('Processed') }}</f7-button>
                <f7-button :active="status === InboxItemStatus.Discarded" @click="changeStatus(InboxItemStatus.Discarded)">{{ tt('Discarded') }}</f7-button>
            </f7-segmented>
        </f7-block>

        <f7-block class="margin-top-half">
            <f7-button fill round icon-f7="doc_on_doc" :class="{ 'disabled': uploading }"
                       @click="openStatementPicker">{{ tt('Upload Statement') }}</f7-button>
        </f7-block>

        <f7-list strong inset dividers class="margin-top-half skeleton-text" v-if="loading">
            <f7-list-item title="Inbox item" :key="itemIdx" v-for="itemIdx in [ 1, 2, 3 ]">
                <template #media>
                    <f7-icon f7="doc_text"></f7-icon>
                </template>
            </f7-list-item>
        </f7-list>

        <f7-list strong inset dividers class="margin-top-half" v-else-if="!items.length">
            <f7-list-item :title="status === InboxItemStatus.Pending ? tt('Process your receipts from your CLI with /recibos') : tt('No items')"></f7-list-item>
        </f7-list>

        <f7-list strong inset dividers class="margin-top-half inbox-list" v-else>
            <f7-list-item link="#" :key="item.id" v-for="item in items"
                          :title="getItemTitle(item)"
                          :after="formatCreatedTime(item)"
                          :footer="item.processedNote || undefined"
                          @click="openItemActions(item)">
                <template #media>
                    <img class="inbox-item-thumbnail" alt="" :src="inboxStore.getItemPictureUrl(item)"
                         v-if="item.kind === InboxItemKind.Receipt && item.pictureUrl" />
                    <f7-icon f7="doc_text" v-else></f7-icon>
                </template>
            </f7-list-item>
        </f7-list>

        <f7-block class="margin-top-half" v-if="!loading && items.length < totalCount">
            <f7-button outline round :class="{ 'disabled': loadingMore }" @click="loadMore">{{ tt('Load More') }}</f7-button>
        </f7-block>

        <f7-actions close-by-outside-click close-on-escape :opened="showItemActions" @actions:closed="showItemActions = false">
            <f7-actions-group>
                <f7-actions-label v-if="selectedItem">{{ getItemTitle(selectedItem) }}</f7-actions-label>
                <f7-actions-button close v-if="selectedItem && selectedItem.status === InboxItemStatus.Pending" @click="discard">{{ tt('Discard') }}</f7-actions-button>
                <f7-actions-button close v-if="selectedItem && selectedItem.status !== InboxItemStatus.Pending" @click="reopen">{{ tt('Reopen') }}</f7-actions-button>
                <f7-actions-button close :key="transactionId" v-for="(transactionId, index) in selectedTransactionIds" @click="openTransaction(transactionId)">{{ tt('Transaction') }} {{ index + 1 }}</f7-actions-button>
                <f7-actions-button close color="red" v-if="selectedItem && selectedItem.status !== InboxItemStatus.Processed" @click="remove">{{ tt('Delete') }}</f7-actions-button>
            </f7-actions-group>
            <f7-actions-group>
                <f7-actions-button bold close>{{ tt('Cancel') }}</f7-actions-button>
            </f7-actions-group>
        </f7-actions>

        <input ref="statementInput" type="file" style="display: none" :accept="INBOX_STATEMENT_FILE_ACCEPT" @change="onStatementSelected($event)" />
    </f7-page>
</template>

<script setup lang="ts">
import { ref, computed, useTemplateRef } from 'vue';
import type { Router } from 'framework7/types';

import { useI18n } from '@/locales/helpers.ts';
import { useI18nUIComponents, showLoading, hideLoading } from '@/lib/ui/mobile.ts';

import { useInboxStore } from '@/stores/inbox.ts';

import {
    InboxItem,
    InboxItemKind,
    InboxItemStatus,
    INBOX_PAGE_SIZE,
    INBOX_STATEMENT_FILE_ACCEPT
} from '@/models/inbox.ts';

import { parseDateTimeFromUnixTime } from '@/lib/datetime.ts';

const props = defineProps<{
    f7router: Router.Router;
}>();

const { tt, formatDateTimeToShortDateTime } = useI18n();
const { showToast, showConfirm } = useI18nUIComponents();

const inboxStore = useInboxStore();

const statementInput = useTemplateRef<HTMLInputElement>('statementInput');

const status = ref<InboxItemStatus>(InboxItemStatus.Pending);
const items = ref<InboxItem[]>([]);
const totalCount = ref<number>(0);
const currentPage = ref<number>(1);
const loading = ref<boolean>(true);
const loadingMore = ref<boolean>(false);
const uploading = ref<boolean>(false);
const selectedItem = ref<InboxItem | null>(null);
const showItemActions = ref<boolean>(false);

// a pending item with rows already created (partial progress) also links to its transactions, as the desktop does
const selectedTransactionIds = computed<string[]>(() => selectedItem.value ? selectedItem.value.resultTransactionIds : []);

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
            showToast(error.message || error);
        }
    });
}

function reload(done?: () => void): void {
    fetchPage(1).then(() => {
        done?.();
    }).catch(error => {
        done?.();

        if (!error.processed) {
            showToast(error.message || error);
        }
    });
}

function changeStatus(newStatus: InboxItemStatus): void {
    if (status.value === newStatus) {
        return;
    }

    status.value = newStatus;
    items.value = [];
    totalCount.value = 0;
    loading.value = true;

    fetchPage(1).then(() => {
        loading.value = false;
    }).catch(error => {
        loading.value = false;

        if (!error.processed) {
            showToast(error.message || error);
        }
    });
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
            showToast(error.message || error);
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
    showLoading();

    inboxStore.uploadItem({
        file: file,
        kind: InboxItemKind.Statement
    }).then(() => {
        uploading.value = false;
        hideLoading();

        showToast(tt('Statement uploaded'));

        // the new statement is pending, so show the pending list
        if (status.value !== InboxItemStatus.Pending) {
            changeStatus(InboxItemStatus.Pending);
        } else {
            reload();
        }
    }).catch(error => {
        uploading.value = false;
        hideLoading();

        if (!error.processed) {
            showToast(error.message || error);
        }
    });
}

function getItemTitle(item: InboxItem): string {
    if (item.kind === InboxItemKind.Statement) {
        return item.originalFileName || item.note || tt('Statement');
    }

    return item.note || tt('Receipt');
}

function formatCreatedTime(item: InboxItem): string {
    return formatDateTimeToShortDateTime(parseDateTimeFromUnixTime(item.createdTime));
}

function openItemActions(item: InboxItem): void {
    selectedItem.value = item;
    showItemActions.value = true;
}

function discard(): void {
    const item = selectedItem.value;

    if (!item) {
        return;
    }

    inboxStore.discardItem({ item: item }).then(() => {
        reload();
    }).catch(error => {
        if (!error.processed) {
            showToast(error.message || error);
        }
    });
}

function reopen(): void {
    const item = selectedItem.value;

    if (!item) {
        return;
    }

    inboxStore.reopenItem({ item: item }).then(() => {
        reload();
    }).catch(error => {
        if (!error.processed) {
            showToast(error.message || error);
        }
    });
}

function remove(): void {
    const item = selectedItem.value;

    if (!item) {
        return;
    }

    showConfirm('Are you sure you want to delete this item?', () => {
        inboxStore.deleteItem({ item: item }).then(() => {
            reload();
        }).catch(error => {
            if (!error.processed) {
                showToast(error.message || error);
            }
        });
    });
}

function openTransaction(transactionId: string): void {
    props.f7router.navigate(`/transaction/detail?id=${transactionId}`);
}

function onPageAfterIn(): void {
    if (!loading.value) {
        reload();
    }
}

init();
</script>

<style>
.inbox-item-thumbnail {
    width: 44px;
    height: 44px;
    object-fit: cover;
    border-radius: 6px;
}
</style>
