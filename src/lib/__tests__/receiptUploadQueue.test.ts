import { describe, expect, it, vi } from 'vitest';

import { uploadReceiptQueue } from '@/lib/receiptUploadQueue.ts';

interface TestPhoto {
    readonly id: string;
}

function createQueue(ids: string[]) {
    const photos: TestPhoto[] = ids.map(id => ({ id }));

    return {
        photos,
        getNextPhoto: () => photos[0],
        removePhoto: (photo: TestPhoto) => {
            const index = photos.findIndex(p => p.id === photo.id);

            if (index >= 0) {
                photos.splice(index, 1);
            }
        }
    };
}

describe('uploadReceiptQueue', () => {
    it('uploads each photo exactly once and leaves the queue empty', async () => {
        const queue = createQueue([ 'a', 'b', 'c' ]);
        const uploadPhoto = vi.fn<(photo: TestPhoto) => Promise<void>>(() => Promise.resolve());
        const onPhotoUploaded = vi.fn();

        const uploadedCount = await uploadReceiptQueue({ ...queue, uploadPhoto, onPhotoUploaded });

        expect(uploadedCount).toBe(3);
        expect(uploadPhoto).toHaveBeenCalledTimes(3);
        expect(uploadPhoto.mock.calls.map(call => call[0].id)).toEqual([ 'a', 'b', 'c' ]);
        expect(queue.photos).toEqual([]);
        expect(onPhotoUploaded.mock.calls.map(call => call[0])).toEqual([ 1, 2, 3 ]);
    });

    it('does nothing with an empty queue', async () => {
        const queue = createQueue([]);
        const uploadPhoto = vi.fn<(photo: TestPhoto) => Promise<void>>(() => Promise.resolve());

        expect(await uploadReceiptQueue({ ...queue, uploadPhoto })).toBe(0);
        expect(uploadPhoto).not.toHaveBeenCalled();
    });

    it('stops at the first error, keeps the failed photo and the ones after it, and does not repeat the first', async () => {
        const queue = createQueue([ 'a', 'b', 'c' ]);
        const uploadPhoto = vi.fn((photo: TestPhoto) => photo.id === 'b' ? Promise.reject(new Error('network')) : Promise.resolve());
        const onPhotoUploaded = vi.fn();

        await expect(uploadReceiptQueue({ ...queue, uploadPhoto, onPhotoUploaded })).rejects.toThrow('network');

        expect(uploadPhoto.mock.calls.map(call => call[0].id)).toEqual([ 'a', 'b' ]);
        expect(queue.photos.map(photo => photo.id)).toEqual([ 'b', 'c' ]);
        expect(onPhotoUploaded).toHaveBeenCalledTimes(1);

        // the retry uploads the rest, and not the first photo again
        uploadPhoto.mockImplementation(() => Promise.resolve());

        expect(await uploadReceiptQueue({ ...queue, uploadPhoto })).toBe(2);
        expect(uploadPhoto.mock.calls.map(call => call[0].id)).toEqual([ 'a', 'b', 'b', 'c' ]);
        expect(queue.photos).toEqual([]);
    });

    it('stops instead of looping when an uploaded photo cannot be removed (BUG-01)', async () => {
        const queue = createQueue([ 'a', 'b' ]);
        const uploadPhoto = vi.fn<(photo: TestPhoto) => Promise<void>>(() => Promise.resolve());

        await expect(uploadReceiptQueue({ ...queue, removePhoto: () => {}, uploadPhoto })).rejects.toThrow('still in the queue');

        expect(uploadPhoto).toHaveBeenCalledTimes(1);
    });
});
