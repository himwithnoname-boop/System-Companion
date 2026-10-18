#include "companion.h"
#include <string.h>
#include <stdlib.h>
#include <stdio.h>
#include "tinyusb.h"
#include "esp_partition.h"
#include "esp_timer.h"
#include "esp_log.h"
#include "wear_levelling.h"
#include "driver/spi_master.h"
#include "driver/sdspi_host.h"
#include "sdmmc_cmd.h"
#include "ff.h"
#include "diskio_impl.h"
#include "diskio_sdmmc.h"
#include "freertos/FreeRTOS.h"
#include "freertos/task.h"
#include "freertos/semphr.h"

// One MSC interface, two independent removable logical units. The client
// volume is always present and read-only; the optional card is host-owned.
static wl_handle_t flash = WL_INVALID_HANDLE;
static uint32_t flash_blocks;
static uint16_t flash_sector;
static sdmmc_host_t sd_host;
static sdmmc_card_t card;
static SemaphoreHandle_t sd_lock;
static bool sd_present, sd_ejected, sd_changed, flash_ejected;
static bool stopping;
static char sd_label[24] = "unavailable";
static uint8_t sector[512] __attribute__((aligned(4)));

uint32_t storage_flash_bytes(void) { return flash_blocks * flash_sector; }
bool storage_sd_ready(void) {
    if (!sd_lock || xSemaphoreTake(sd_lock, pdMS_TO_TICKS(30)) != pdTRUE) return false;
    bool ready = sd_present && !sd_ejected;
    xSemaphoreGive(sd_lock); return ready;
}
uint64_t storage_sd_bytes(void) {
    if (!sd_lock || xSemaphoreTake(sd_lock, pdMS_TO_TICKS(30)) != pdTRUE) return 0;
    uint64_t bytes = sd_present ? (uint64_t)card.csd.capacity * card.csd.sector_size : 0;
    xSemaphoreGive(sd_lock); return bytes;
}
const char *storage_sd_label(void) {
    static char snapshot[sizeof(sd_label)];
    if (sd_lock && xSemaphoreTake(sd_lock, pdMS_TO_TICKS(30)) == pdTRUE) {
        memcpy(snapshot, sd_label, sizeof(snapshot)); xSemaphoreGive(sd_lock);
    } else snprintf(snapshot, sizeof(snapshot), "unavailable");
    return snapshot;
}

static void label_card(void) {
    // Rename a recognized FAT volume before presenting it to the host.
    // Never create a filesystem or mount it concurrently with the host.
    BYTE drive;
    if (ff_diskio_get_drive(&drive) != ESP_OK) return;
    ff_diskio_register_sdmmc(drive, &card);
    char path[8]; snprintf(path, sizeof(path), "%u:", drive);
    FATFS *fs = calloc(1, sizeof(FATFS));
    if (fs && f_mount(fs, path, 1) == FR_OK) {
        char name[24]; snprintf(name, sizeof(name), "%sMONISTORE", path);
        char current[64] = {0};
        FRESULT result = f_getlabel(path, current, NULL);
        if (result == FR_OK && strcmp(current, "MONISTORE")) result = f_setlabel(name);
        snprintf(sd_label, sizeof(sd_label), "%s", result == FR_OK ? "MONISTORE" : "label unavailable");
    } else snprintf(sd_label, sizeof(sd_label), "not FAT formatted");
    if (fs) f_mount(NULL, path, 0);
    ff_diskio_unregister(drive); free(fs);
}
static bool initialize_card(void) {
    memset(&card, 0, sizeof(card));
    if (sdmmc_card_init(&sd_host, &card) != ESP_OK || card.csd.sector_size != 512) return false;
    label_card(); sd_present = true; sd_ejected = false; return true;
}
static void card_worker(void *unused) {
    for (;;) {
        vTaskDelay(pdMS_TO_TICKS(3000));
        xSemaphoreTake(sd_lock, portMAX_DELAY);
        if(stopping) {xSemaphoreGive(sd_lock);vTaskDelete(NULL);}
        if (sd_present) {
            if (sdmmc_get_status(&card) != ESP_OK) {
                sd_present = false; sd_ejected = false;
                snprintf(sd_label, sizeof(sd_label), "unavailable");
            }
        } else if (initialize_card()) sd_changed = true;
        xSemaphoreGive(sd_lock);
    }
}
void storage_init(void) {
    const esp_partition_t *partition = esp_partition_find_first(ESP_PARTITION_TYPE_DATA, ESP_PARTITION_SUBTYPE_DATA_FAT, "storage");
    assert(partition);
    ESP_ERROR_CHECK(wl_mount(partition, &flash));
    flash_sector = wl_sector_size(flash); flash_blocks = wl_size(flash) / flash_sector;
    sd_lock = xSemaphoreCreateMutex(); assert(sd_lock);
    spi_bus_config_t bus = {.mosi_io_num=39, .miso_io_num=40, .sclk_io_num=41, .quadwp_io_num=-1, .quadhd_io_num=-1, .max_transfer_sz=4096};
    if (spi_bus_initialize(SPI3_HOST, &bus, SPI_DMA_CH_AUTO) != ESP_OK) return;
    sd_host = (sdmmc_host_t)SDSPI_HOST_DEFAULT(); sd_host.slot = SPI3_HOST;
    sdspi_device_config_t slot = SDSPI_DEVICE_CONFIG_DEFAULT(); slot.host_id = SPI3_HOST; slot.gpio_cs = 38;
    sdspi_dev_handle_t handle;
    if (sdspi_host_init() != ESP_OK || sdspi_host_init_device(&slot, &handle) != ESP_OK) return;
    sd_host.slot = handle;
    initialize_card();
    assert(xTaskCreate(card_worker, "sd_card", 6144, NULL, 1, NULL) == pdPASS);
}
void storage_prepare_sleep(void) {
    xSemaphoreTake(sd_lock,portMAX_DELAY);
    /* Taking the same mutex as every SD transaction drains an in-flight write.
     * Reject later I/O before the TinyUSB task is stopped. */
    stopping=true;sd_ejected=true;flash_ejected=true;
    xSemaphoreGive(sd_lock);
}

// This TinyUSB version expects the count, then subtracts one for the wire.
uint8_t tud_msc_get_maxlun_cb(void) { return 2; }
void tud_mount_cb(void) {
    // A new USB host/configuration must see media previously ejected by the
    // old host. The card remains raw and is never mounted in firmware here.
    flash_ejected = false;
    if (xSemaphoreTake(sd_lock, pdMS_TO_TICKS(100)) == pdTRUE) {
        sd_ejected = false; xSemaphoreGive(sd_lock);
    }
}
void tud_msc_inquiry_cb(uint8_t lun, uint8_t vendor[8], uint8_t product[16], uint8_t rev[4]) {
    memset(vendor, ' ', 8); memset(product, ' ', 16); memset(rev, ' ', 4);
    memcpy(vendor, "Monitor", 7);
    const char *name = lun == 0 ? "SYSTEM_MONITOR" : "MoniStore";
    memcpy(product, name, strlen(name)); memcpy(rev, "1.03", 4);
}
bool tud_msc_test_unit_ready_cb(uint8_t lun) {
    if (lun == 0 && !flash_ejected) return true;
    if (lun == 1 && xSemaphoreTake(sd_lock, pdMS_TO_TICKS(100)) == pdTRUE) {
        bool ready = sd_present && !sd_ejected, changed = sd_changed;
        if (ready && changed) sd_changed = false;
        xSemaphoreGive(sd_lock);
        if (ready && !changed) return true;
        if (ready) { tud_msc_set_sense(lun, SCSI_SENSE_UNIT_ATTENTION, 0x28, 0); return false; }
    }
    tud_msc_set_sense(lun, SCSI_SENSE_NOT_READY, 0x3a, 0); return false;
}
void tud_msc_capacity_cb(uint8_t lun, uint32_t *count, uint16_t *size) {
    *count = 0; *size = 512;
    if (lun == 0) { *count = flash_blocks; *size = flash_sector; }
    else if (lun == 1 && xSemaphoreTake(sd_lock, pdMS_TO_TICKS(100)) == pdTRUE) {
        if (sd_present) { *count = card.csd.capacity; *size = card.csd.sector_size; }
        xSemaphoreGive(sd_lock);
    }
}
bool tud_msc_is_writable_cb(uint8_t lun) { return lun == 1 && storage_sd_ready(); }
bool tud_msc_start_stop_cb(uint8_t lun, uint8_t power, bool start, bool eject) {
    if (lun > 1) return false;
    if (eject) {
        if (lun == 0) flash_ejected = !start;
        else if (xSemaphoreTake(sd_lock, pdMS_TO_TICKS(250)) == pdTRUE) {
            sd_ejected = !start; xSemaphoreGive(sd_lock);
        } else return false;
    }
    return true;
}
static int32_t sd_transfer(uint32_t lba, uint32_t offset, void *buffer, uint32_t length, bool write) {
    if (xSemaphoreTake(sd_lock, pdMS_TO_TICKS(250)) != pdTRUE) return -1;
    uint64_t address = (uint64_t)lba * 512 + offset;
    esp_err_t result = ESP_ERR_INVALID_SIZE;
    if (!stopping && sd_present && !sd_ejected && address + length <= (uint64_t)card.csd.capacity * 512) {
        result = ESP_OK;
        uint8_t *bytes = buffer;
        while (length && result == ESP_OK) {
            uint32_t block = address / 512, skip = address % 512;
            uint32_t n = length < 512-skip ? length : 512-skip;
            // Aligned runs use one driver transaction; partial sectors use RMW.
            if (!skip && length >= 512) {
                n = (length / 512) * 512;
                result = write ? sdmmc_write_sectors(&card, bytes, block, n/512) : sdmmc_read_sectors(&card, bytes, block, n/512);
            } else {
                result = sdmmc_read_sectors(&card, sector, block, 1);
                if (result == ESP_OK) {
                    if (write) { memcpy(sector + skip, bytes, n); result = sdmmc_write_sectors(&card, sector, block, 1); }
                    else memcpy(bytes, sector + skip, n);
                }
            }
            address += n; bytes += n; length -= n;
        }
        if (result != ESP_OK) sd_present = false;
    }
    xSemaphoreGive(sd_lock); return result == ESP_OK ? 0 : -1;
}
int32_t tud_msc_read10_cb(uint8_t lun, uint32_t lba, uint32_t offset, void *buffer, uint32_t size) {
    if (lun == 0 && !flash_ejected && (uint64_t)lba * flash_sector + offset + size <= storage_flash_bytes()) {
        if (wl_read(flash, (size_t)lba * flash_sector + offset, buffer, size) == ESP_OK) return size;
    } else if (lun == 1 && sd_transfer(lba, offset, buffer, size, false) == 0) return size;
    tud_msc_set_sense(lun, SCSI_SENSE_MEDIUM_ERROR, 0x11, 0); return -1;
}
int32_t tud_msc_write10_cb(uint8_t lun, uint32_t lba, uint32_t offset, uint8_t *buffer, uint32_t size) {
    if (lun == 0) { tud_msc_set_sense(lun, SCSI_SENSE_DATA_PROTECT, 0x27, 0); return -1; }
    if (lun == 1 && sd_transfer(lba, offset, buffer, size, true) == 0) return size;
    tud_msc_set_sense(lun, SCSI_SENSE_MEDIUM_ERROR, 0x0c, 0); return -1;
}
int32_t tud_msc_scsi_cb(uint8_t lun, const uint8_t command[16], void *buffer, uint16_t size) {
    // Writes finish synchronously before acknowledgement; no firmware cache
    // remains to flush. Host filesystem caches still require normal eject.
    if (lun <= 1 && (command[0] == SCSI_CMD_PREVENT_ALLOW_MEDIUM_REMOVAL || command[0] == 0x35)) return 0;
    tud_msc_set_sense(lun, SCSI_SENSE_ILLEGAL_REQUEST, 0x20, 0); return -1;
}
