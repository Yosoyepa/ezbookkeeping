import { beforeEach, describe, expect, it, vi } from 'vitest';
import { createPinia, setActivePinia } from 'pinia';

import { useInboxStore } from '@/stores/inbox.ts';

import {
    type InboxItemInfoResponse,
    InboxItem,
    InboxItemKind,
    InboxItemStatus,
    INBOX_RECEIPT_MAX_FILE_SIZE
} from '@/models/inbox.ts';

import services from '@/lib/services.ts';

// The services read window.location and localStorage when they are loaded and the test environment is node
vi.hoisted(() => {
    const data = new Map<string, string>();
    const storage = {
        getItem: (key: string) => data.has(key) ? data.get(key) as string : null,
        setItem: (key: string, value: string) => { data.set(key, String(value)); },
        removeItem: (key: string) => { data.delete(key); },
        clear: () => { data.clear(); },
        key: (index: number) => Array.from(data.keys())[index] ?? null,
        get length() { return data.size; }
    };

    Object.assign(globalThis, {
        window: { location: { pathname: '/', origin: 'http://localhost', href: 'http://localhost/' }, localStorage: storage, sessionStorage: storage },
        localStorage: storage,
        sessionStorage: storage
    });
});

function createItemResponse(overrides: Partial<InboxItemInfoResponse> = {}): InboxItemInfoResponse {
    return {
        id: '1',
        kind: InboxItemKind.Receipt,
        status: InboxItemStatus.Pending,
        pictureId: '11',
        pictureUrl: '/pictures/11.jpg?token=test',
        fileExtension: 'jpg',
        note: 'Lunch',
        resultTransactionIds: [],
        createdTime: 1793404800,
        ...overrides
    };
}

describe('inbox store', () => {
    beforeEach(() => {
        setActivePinia(createPinia());
    });

    it('lists the items of the requested status and page', async () => {
        const getInboxItems = vi.spyOn(services, 'getInboxItems').mockResolvedValue({
            data: {
                success: true,
                result: {
                    items: [
                        createItemResponse({ id: '1' }),
                        createItemResponse({ id: '2', kind: InboxItemKind.Statement, pictureUrl: undefined, note: undefined, originalFileName: 'bancolombia.pdf', fileExtension: 'pdf' })
                    ],
                    totalCount: 3
                }
            }
        } as never);

        const inboxStore = useInboxStore();
        const page = await inboxStore.loadItems({ status: InboxItemStatus.Pending, count: 20, page: 1 });

        expect(getInboxItems).toHaveBeenCalledWith({ status: InboxItemStatus.Pending, count: 20, page: 1 });
        expect(page.totalCount).toBe(3);
        expect(page.items).toHaveLength(2);
        expect(page.items[0]).toBeInstanceOf(InboxItem);
        expect(page.items[1]?.originalFileName).toBe('bancolombia.pdf');
    });

    it('uploads a receipt and counts it as pending', async () => {
        const uploadInboxItem = vi.spyOn(services, 'uploadInboxItem').mockResolvedValue({
            data: { success: true, result: createItemResponse({ id: '5' }) }
        } as never);

        const inboxStore = useInboxStore();
        const file = new File(['image'], 'receipt.jpg', { type: 'image/jpeg' });
        const item = await inboxStore.uploadItem({ file: file, kind: InboxItemKind.Receipt });

        expect(uploadInboxItem.mock.calls[0]?.[0]).toMatchObject({ file: file, kind: InboxItemKind.Receipt });
        expect(item.id).toBe('5');
        expect(inboxStore.pendingReceiptCount).toBe(1);
        expect(inboxStore.pendingItemCount).toBe(1);
    });

    it('rejects a receipt larger than 10 MB without calling the API', async () => {
        const uploadInboxItem = vi.spyOn(services, 'uploadInboxItem');

        const inboxStore = useInboxStore();
        const file = new File(['image'], 'large.jpg', { type: 'image/jpeg' });
        Object.defineProperty(file, 'size', { value: INBOX_RECEIPT_MAX_FILE_SIZE + 1 });

        await expect(inboxStore.uploadItem({ file: file, kind: InboxItemKind.Receipt })).rejects.toEqual({ message: 'Receipt is too large' });
        expect(uploadInboxItem).not.toHaveBeenCalled();
    });

    it('discards a pending item and lowers the pending count', async () => {
        vi.spyOn(services, 'getInboxItemStats').mockResolvedValue({
            data: { success: true, result: { pendingReceipts: 2, pendingStatements: 1 } }
        } as never);
        const discardInboxItem = vi.spyOn(services, 'discardInboxItem').mockResolvedValue({
            data: { success: true, result: true }
        } as never);

        const inboxStore = useInboxStore();
        await inboxStore.loadStats();

        expect(inboxStore.pendingItemCount).toBe(3);

        const item = InboxItem.of(createItemResponse({ id: '7' }));
        const discarded = await inboxStore.discardItem({ item: item, note: 'duplicate' });

        expect(discarded).toBe(true);
        expect(discardInboxItem).toHaveBeenCalledWith({ id: '7', note: 'duplicate' });
        expect(inboxStore.pendingReceiptCount).toBe(1);
        expect(inboxStore.pendingItemCount).toBe(2);
    });
});
