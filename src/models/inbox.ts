export enum InboxItemKind {
    Receipt = 1,
    Statement = 2
}

export enum InboxItemStatus {
    Pending = 1,
    Processed = 2,
    Discarded = 3
}

export const INBOX_PAGE_SIZE: number = 20;
export const INBOX_RECEIPT_MAX_FILE_SIZE: number = 10 * 1024 * 1024; // 10 MB
export const INBOX_STATEMENT_MAX_FILE_SIZE: number = 20 * 1024 * 1024; // 20 MB
export const INBOX_STATEMENT_FILE_ACCEPT: string = '.pdf,.csv,.xlsx,.xls,.txt,.ofx,.qif';

export interface InboxItemInfoResponse {
    readonly id: string;
    readonly kind: InboxItemKind;
    readonly status: InboxItemStatus;
    readonly pictureId?: string;
    readonly pictureUrl?: string;
    readonly fileExtension: string;
    readonly originalFileName?: string;
    readonly note?: string;
    readonly resultTransactionIds: string[];
    readonly processedNote?: string;
    readonly createdTime: number;
    readonly processedTime?: number;
}

export interface InboxItemListResponse {
    readonly items: InboxItemInfoResponse[];
    readonly totalCount: number;
}

export interface InboxItemStatsResponse {
    readonly pendingReceipts: number;
    readonly pendingStatements: number;
}

export interface InboxItemListRequest {
    readonly status?: InboxItemStatus;
    readonly kind?: InboxItemKind;
    readonly count: number;
    readonly page: number;
}

export interface InboxItemIdRequest {
    readonly id: string;
}

export interface InboxItemDiscardRequest extends InboxItemIdRequest {
    readonly note?: string;
}

export interface InboxItemPage {
    readonly items: InboxItem[];
    readonly totalCount: number;
}

export class InboxItem implements InboxItemInfoResponse {
    public id: string;
    public kind: InboxItemKind;
    public status: InboxItemStatus;
    public pictureId?: string;
    public pictureUrl?: string;
    public fileExtension: string;
    public originalFileName?: string;
    public note?: string;
    public resultTransactionIds: string[];
    public processedNote?: string;
    public createdTime: number;
    public processedTime?: number;

    private constructor(response: InboxItemInfoResponse) {
        this.id = response.id;
        this.kind = response.kind;
        this.status = response.status;
        this.pictureId = response.pictureId;
        this.pictureUrl = response.pictureUrl;
        this.fileExtension = response.fileExtension;
        this.originalFileName = response.originalFileName;
        this.note = response.note;
        this.resultTransactionIds = response.resultTransactionIds || [];
        this.processedNote = response.processedNote;
        this.createdTime = response.createdTime;
        this.processedTime = response.processedTime;
    }

    public static of(response: InboxItemInfoResponse): InboxItem {
        return new InboxItem(response);
    }

    public static ofMulti(responses: InboxItemInfoResponse[]): InboxItem[] {
        const items: InboxItem[] = [];

        for (const response of responses) {
            items.push(InboxItem.of(response));
        }

        return items;
    }
}
