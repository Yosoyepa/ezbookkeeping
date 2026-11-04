<template>
    <f7-sheet swipe-handler=".swipe-handler" style="height:auto"
              :swipe-to-close="!uploading"
              :opened="show" @sheet:closed="onSheetClosed">
        <f7-toolbar class="toolbar-with-swipe-handler">
            <div class="swipe-handler"></div>
            <div class="left">
                <f7-link icon-f7="xmark" :class="{ 'disabled': uploading }"
                         :aria-label="tt('Cancel')"
                         @click="cancel"></f7-link>
            </div>
        </f7-toolbar>
        <f7-page-content class="no-margin-vertical no-padding-vertical">
            <f7-block-title class="receipt-capture-title">{{ tt('New Receipts') }}</f7-block-title>
            <f7-block class="margin-top-half">
                <div class="receipt-capture-grid">
                    <div class="receipt-capture-item" :key="photo.id" v-for="photo in photos">
                        <img class="receipt-capture-thumbnail" alt="" :src="photo.previewUrl" />
                        <f7-link class="receipt-capture-remove" icon-f7="xmark_circle_fill"
                                 :class="{ 'disabled': uploading }"
                                 :aria-label="tt('Remove')"
                                 @click="removePhoto(photo.id)"></f7-link>
                    </div>
                </div>
            </f7-block>
            <f7-block class="margin-top-half" v-if="uploading">
                <f7-progressbar :progress="uploadPercent"></f7-progressbar>
                <p class="text-align-center margin-top-half">{{ tt('Uploading receipts') }} ({{ uploadedCount }}/{{ uploadTotal }})</p>
            </f7-block>
            <f7-block class="margin-top-half receipt-capture-actions">
                <f7-button outline round icon-f7="camera" :class="{ 'disabled': uploading }"
                           @click="openCamera">{{ tt('Take Another Photo') }}</f7-button>
                <f7-button fill round icon-f7="checkmark_alt" :class="{ 'disabled': uploading || !photos.length }"
                           @click="upload">{{ tt('Upload') }} ({{ photos.length }})</f7-button>
            </f7-block>
        </f7-page-content>

        <input ref="receiptInput" type="file" style="display: none" accept="image/*" capture="environment" multiple @change="onReceiptSelected($event)" />
    </f7-sheet>
</template>

<script setup lang="ts">
import { ref, computed, useTemplateRef } from 'vue';

import { useI18n } from '@/locales/helpers.ts';
import { useI18nUIComponents } from '@/lib/ui/mobile.ts';

import { useInboxStore } from '@/stores/inbox.ts';

import { ImageUploadQualityType } from '@/core/image.ts';
import { KnownFileType } from '@/core/file.ts';

import { InboxItemKind } from '@/models/inbox.ts';

import { generateRandomUUID } from '@/lib/misc.ts';
import { compressJpgImageByQuality } from '@/lib/ui/common.ts';
import logger from '@/lib/logger.ts';

interface ReceiptPhoto {
    readonly id: string;
    readonly file: File;
    readonly previewUrl: string;
}

defineProps<{
    show: boolean;
}>();

const emit = defineEmits<{
    (e: 'update:show', value: boolean): void;
}>();

const { tt } = useI18n();
const { showToast } = useI18nUIComponents();

const inboxStore = useInboxStore();

const receiptInput = useTemplateRef<HTMLInputElement>('receiptInput');

const photos = ref<ReceiptPhoto[]>([]);
const uploading = ref<boolean>(false);
const uploadedCount = ref<number>(0);
const uploadTotal = ref<number>(0);
const uploadFraction = ref<number>(0);

const uploadPercent = computed<number>(() => uploadTotal.value > 0 ? ((uploadedCount.value + uploadFraction.value) / uploadTotal.value) * 100 : 0);

function openCamera(): void {
    if (uploading.value) {
        return;
    }

    receiptInput.value?.click();
}

function onReceiptSelected(event: Event): void {
    const el = event.target as HTMLInputElement;

    if (!el.files || !el.files.length) {
        return;
    }

    for (const file of Array.from(el.files)) {
        photos.value.push({
            id: generateRandomUUID(),
            file: file,
            previewUrl: URL.createObjectURL(file)
        });
    }

    // clear the input so that the same photo can be selected again
    el.value = '';

    emit('update:show', true);
}

function removePhoto(photoId: string): void {
    if (uploading.value) {
        return;
    }

    const index = photos.value.findIndex(photo => photo.id === photoId);

    if (index < 0) {
        return;
    }

    const [photo] = photos.value.splice(index, 1);

    if (photo) {
        URL.revokeObjectURL(photo.previewUrl);
    }
}

function clearPhotos(): void {
    for (const photo of photos.value) {
        URL.revokeObjectURL(photo.previewUrl);
    }

    photos.value = [];
}

function compressReceipt(file: File): Promise<File> {
    return compressJpgImageByQuality(file, ImageUploadQualityType.FHD1080P).then(blob => {
        return KnownFileType.JPG.createFileFromBlob(blob, 'receipt');
    }).catch(error => {
        logger.warn('failed to compress receipt image, uploading the original file', error);
        return file;
    });
}

function upload(): void {
    if (uploading.value || !photos.value.length) {
        return;
    }

    uploading.value = true;
    uploadedCount.value = 0;
    uploadTotal.value = photos.value.length;

    uploadNextPhoto();
}

// Photos are uploaded one by one. A failed photo stays in the list so the user can retry.
function uploadNextPhoto(): void {
    const photo = photos.value[0];

    if (!photo) {
        finishUpload();
        return;
    }

    uploadFraction.value = 0;

    compressReceipt(photo.file).then(file => {
        return inboxStore.uploadItem({
            file: file,
            kind: InboxItemKind.Receipt,
            onUploadProgress: (progress: number) => {
                uploadFraction.value = progress;
            }
        });
    }).then(() => {
        removePhoto(photo.id);
        uploadedCount.value++;
        uploadNextPhoto();
    }).catch(error => {
        uploading.value = false;
        uploadFraction.value = 0;

        if (!error.processed) {
            showToast(error.message || error);
        }
    });
}

function finishUpload(): void {
    const count = uploadTotal.value;

    uploading.value = false;
    uploadFraction.value = 0;

    emit('update:show', false);

    if (count === 1) {
        showToast(tt('One receipt in the inbox'));
    } else {
        showToast(tt('{count} receipts in the inbox', { count: count }));
    }
}

function cancel(): void {
    if (uploading.value) {
        return;
    }

    emit('update:show', false);
}

function onSheetClosed(): void {
    if (!uploading.value) {
        clearPhotos();
    }

    emit('update:show', false);
}

defineExpose({
    openCamera
});
</script>

<style>
/* the close button sits on the same row, so the title starts after it */
.block-title.receipt-capture-title {
    margin-inline-start: 72px;
}

.receipt-capture-actions {
    display: flex;
    gap: 8px;
}

.receipt-capture-actions > .button {
    flex: 1 1 0;
    min-width: 0;
}

.receipt-capture-grid {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(96px, 1fr));
    gap: 8px;
}

.receipt-capture-item {
    position: relative;
    aspect-ratio: 3 / 4;
}

.receipt-capture-thumbnail {
    width: 100%;
    height: 100%;
    object-fit: cover;
    border-radius: 8px;
    border: 1px solid var(--f7-page-master-border-color);
}

.receipt-capture-item .receipt-capture-remove {
    position: absolute;
    top: 4px;
    inset-inline-end: 4px;
    color: #ffffff;
    text-shadow: 0 0 4px rgba(0, 0, 0, 0.6);
}
</style>
