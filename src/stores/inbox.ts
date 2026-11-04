import { ref, computed } from 'vue';
import { defineStore } from 'pinia';

import {
    type InboxItemListRequest,
    type InboxItemPage,
    type InboxItemStatsResponse,
    InboxItem,
    InboxItemKind,
    InboxItemStatus,
    INBOX_RECEIPT_MAX_FILE_SIZE,
    INBOX_STATEMENT_MAX_FILE_SIZE
} from '@/models/inbox.ts';

import services from '@/lib/services.ts';
import logger from '@/lib/logger.ts';

interface RequestError {
    readonly processed?: boolean;
    readonly response?: {
        readonly data?: {
            readonly errorMessage?: string;
        };
    };
}

function toRejectReason(error: RequestError, message: string): unknown {
    if (error.response && error.response.data && error.response.data.errorMessage) {
        return { error: error.response.data };
    } else if (!error.processed) {
        return { message: message };
    }

    return error;
}

export const useInboxStore = defineStore('inbox', () => {
    const pendingReceiptCount = ref<number>(0);
    const pendingStatementCount = ref<number>(0);
    const pendingItemCount = computed<number>(() => pendingReceiptCount.value + pendingStatementCount.value);

    // Counts are adjusted locally after each change, so the badge does not need another request
    function adjustPendingCount(kind: InboxItemKind, delta: number): void {
        if (kind === InboxItemKind.Receipt) {
            pendingReceiptCount.value = Math.max(0, pendingReceiptCount.value + delta);
        } else {
            pendingStatementCount.value = Math.max(0, pendingStatementCount.value + delta);
        }
    }

    function loadStats(): Promise<InboxItemStatsResponse> {
        return new Promise((resolve, reject) => {
            services.getInboxItemStats().then(response => {
                const data = response.data;

                if (!data || !data.success || !data.result) {
                    reject({ message: 'Unable to retrieve inbox statistics' });
                    return;
                }

                pendingReceiptCount.value = data.result.pendingReceipts;
                pendingStatementCount.value = data.result.pendingStatements;

                resolve(data.result);
            }).catch(error => {
                logger.error('failed to load inbox statistics', error);
                reject(toRejectReason(error, 'Unable to retrieve inbox statistics'));
            });
        });
    }

    function loadItems(req: InboxItemListRequest): Promise<InboxItemPage> {
        return new Promise((resolve, reject) => {
            services.getInboxItems(req).then(response => {
                const data = response.data;

                if (!data || !data.success || !data.result) {
                    reject({ message: 'Unable to retrieve inbox items' });
                    return;
                }

                resolve({
                    items: InboxItem.ofMulti(data.result.items || []),
                    totalCount: data.result.totalCount
                });
            }).catch(error => {
                logger.error('failed to load inbox items', error);
                reject(toRejectReason(error, 'Unable to retrieve inbox items'));
            });
        });
    }

    function uploadItem({ file, kind, note, onUploadProgress }: { file: File, kind: InboxItemKind, note?: string, onUploadProgress?: (progress: number) => void }): Promise<InboxItem> {
        const maxFileSize = kind === InboxItemKind.Receipt ? INBOX_RECEIPT_MAX_FILE_SIZE : INBOX_STATEMENT_MAX_FILE_SIZE;

        if (file.size > maxFileSize) {
            return Promise.reject({ message: kind === InboxItemKind.Receipt ? 'Receipt is too large' : 'Statement is too large' });
        }

        return new Promise((resolve, reject) => {
            services.uploadInboxItem({
                file: file,
                kind: kind,
                note: note,
                onUploadProgress: (event) => {
                    if (onUploadProgress && event.total) {
                        onUploadProgress(event.loaded / event.total);
                    }
                }
            }).then(response => {
                const data = response.data;

                if (!data || !data.success || !data.result) {
                    reject({ message: 'Unable to upload file' });
                    return;
                }

                adjustPendingCount(kind, 1);

                resolve(InboxItem.of(data.result));
            }).catch(error => {
                logger.error('failed to upload inbox item', error);
                reject(toRejectReason(error, 'Unable to upload file'));
            });
        });
    }

    function discardItem({ item, note }: { item: InboxItem, note?: string }): Promise<boolean> {
        return new Promise((resolve, reject) => {
            services.discardInboxItem({
                id: item.id,
                note: note
            }).then(response => {
                const data = response.data;

                if (!data || !data.success || !data.result) {
                    reject({ message: 'Unable to discard item' });
                    return;
                }

                if (item.status === InboxItemStatus.Pending) {
                    adjustPendingCount(item.kind, -1);
                }

                resolve(data.result);
            }).catch(error => {
                logger.error('failed to discard inbox item', error);
                reject(toRejectReason(error, 'Unable to discard item'));
            });
        });
    }

    function reopenItem({ item }: { item: InboxItem }): Promise<boolean> {
        return new Promise((resolve, reject) => {
            services.reopenInboxItem({
                id: item.id
            }).then(response => {
                const data = response.data;

                if (!data || !data.success || !data.result) {
                    reject({ message: 'Unable to reopen item' });
                    return;
                }

                if (item.status !== InboxItemStatus.Pending) {
                    adjustPendingCount(item.kind, 1);
                }

                resolve(data.result);
            }).catch(error => {
                logger.error('failed to reopen inbox item', error);
                reject(toRejectReason(error, 'Unable to reopen item'));
            });
        });
    }

    function deleteItem({ item }: { item: InboxItem }): Promise<boolean> {
        return new Promise((resolve, reject) => {
            services.deleteInboxItem({
                id: item.id
            }).then(response => {
                const data = response.data;

                if (!data || !data.success || !data.result) {
                    reject({ message: 'Unable to delete item' });
                    return;
                }

                if (item.status === InboxItemStatus.Pending) {
                    adjustPendingCount(item.kind, -1);
                }

                resolve(data.result);
            }).catch(error => {
                logger.error('failed to delete inbox item', error);
                reject(toRejectReason(error, 'Unable to delete item'));
            });
        });
    }

    function getItemPictureUrl(item: InboxItem): string {
        return services.getTransactionPictureUrl(item.pictureUrl || '');
    }

    return {
        // states
        pendingReceiptCount,
        pendingStatementCount,
        pendingItemCount,
        // functions
        loadStats,
        loadItems,
        uploadItem,
        discardItem,
        reopenItem,
        deleteItem,
        getItemPictureUrl
    };
});
