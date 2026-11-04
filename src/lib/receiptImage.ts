import { ImageUploadQualityType } from '@/core/image.ts';
import { KnownFileType } from '@/core/file.ts';

import { compressJpgImageByQuality } from '@/lib/ui/common.ts';
import logger from '@/lib/logger.ts';

/**
 * Compresses a receipt image to a 1080p JPG before it is uploaded to the inbox.
 * When the image cannot be compressed, the original file is returned so the upload can still go on.
 */
export function compressReceiptImage(file: File): Promise<File> {
    return compressJpgImageByQuality(file, ImageUploadQualityType.FHD1080P).then(blob => {
        return KnownFileType.JPG.createFileFromBlob(blob, 'receipt');
    }).catch(error => {
        logger.warn('failed to compress receipt image, uploading the original file', error);
        return file;
    });
}
