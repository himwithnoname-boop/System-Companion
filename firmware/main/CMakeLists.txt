idf_component_register(SRCS "main.c" "board.c" "power.c" "model.c" "usb.c" "storage.c" "ui.c" "screens.c" "settings.c" "gpu_profiles.c"
    INCLUDE_DIRS "." REQUIRES lvgl esp_lcd driver esp_timer nvs_flash json fatfs wear_levelling read_lcd_id_bsp mbedtls sdmmc)
fatfs_create_spiflash_image(storage "../../dist/DEVICE" FLASH_IN_PROJECT)
