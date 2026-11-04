export interface ReceiptUploadQueueOptions<T extends { readonly id: string }> {
    /** Returns the photo at the head of the queue, or undefined when the queue is empty. */
    readonly getNextPhoto: () => T | undefined;
    /** Uploads one photo. A rejected promise stops the queue and keeps the photo in it. */
    readonly uploadPhoto: (photo: T) => Promise<void>;
    /** Takes an uploaded photo out of the queue. */
    readonly removePhoto: (photo: T) => void;
    /** Called after each photo that was uploaded and removed, with the number uploaded so far. */
    readonly onPhotoUploaded?: (uploadedCount: number) => void;
}

/**
 * Uploads the photos of the queue one by one, in order, and returns how many were uploaded.
 *
 * Each photo is removed from the queue as soon as it is uploaded, so a failure leaves the failed photo
 * and the ones after it in the queue, and the photos already uploaded are never sent again.
 */
export async function uploadReceiptQueue<T extends { readonly id: string }>(options: ReceiptUploadQueueOptions<T>): Promise<number> {
    let uploadedCount = 0;

    for (;;) {
        const photo = options.getNextPhoto();

        if (!photo) {
            return uploadedCount;
        }

        await options.uploadPhoto(photo);
        options.removePhoto(photo);

        // a photo that stays in the queue after it was uploaded would be uploaded again and again
        if (options.getNextPhoto()?.id === photo.id) {
            throw new Error('the uploaded receipt photo is still in the queue');
        }

        uploadedCount++;
        options.onPhotoUploaded?.(uploadedCount);
    }
}
