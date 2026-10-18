#include "companion.h"
#include <math.h>
#include <stdio.h>
#include <stdlib.h>
#include <time.h>
#include "esp_timer.h"
#include "esp_heap_caps.h"
static lv_obj_t *root,*body,*header,*connection,*pager,*overlay,*clock_label;
static lv_obj_t *power_overlay,*power_label;
static int page,panel;
static bool connected,sleeping,capture_pending;
static uint32_t last_seq;
static int64_t last_touch_us,last_tap_us;
static int down_x,down_y;
static bool dragged,long_pressed,woke_on_press;
static const uint32_t accents[]={0x00e5ff,0x76b900,0xff00aa,0x0066ff,0x00ff66};
static const char *themes[]={"AMOLED","NVIDIA","CYBERPUNK","LIGHT","MATRIX"};
uint32_t ui_theme_bg(void) {return settings.theme==3?0xf5f6f8:0x000000;}
uint32_t ui_theme_fg(void) {return settings.theme==3?0x111820:0xf4f7fa;}
uint32_t ui_accent(void) {return accents[settings.theme%5];}
int ui_page(void) {return page;}
bool ui_is_connected(void) {return connected;}
lv_obj_t *ui_label(lv_obj_t *p,const char *txt,int x,int y,int w,const lv_font_t *font,uint32_t color) {
    lv_obj_t *o=lv_label_create(p);lv_label_set_text(o,txt);lv_obj_set_pos(o,x,y);lv_obj_set_width(o,w);
    lv_obj_set_style_text_font(o,font,0);lv_obj_set_style_text_color(o,lv_color_hex(color),0);
    lv_obj_set_style_text_align(o,LV_TEXT_ALIGN_CENTER,0);lv_label_set_long_mode(o,LV_LABEL_LONG_WRAP);
    lv_obj_add_flag(o,LV_OBJ_FLAG_EVENT_BUBBLE);return o;
}
lv_obj_t *ui_ring(lv_obj_t *p,int size,int y,int width,uint32_t color) {
    lv_obj_t *o=lv_arc_create(p);lv_obj_set_size(o,size,size);lv_obj_set_pos(o,(466-size)/2,y);
    lv_arc_set_rotation(o,135);lv_arc_set_bg_angles(o,0,270);lv_arc_set_range(o,0,100);lv_arc_set_value(o,0);
    lv_obj_remove_style(o,NULL,LV_PART_KNOB);lv_obj_set_style_arc_width(o,width,LV_PART_MAIN);
    lv_obj_set_style_arc_width(o,width,LV_PART_INDICATOR);lv_obj_set_style_arc_color(o,lv_color_hex(color),LV_PART_INDICATOR);
    lv_obj_set_style_arc_color(o,lv_color_hex(settings.theme==3?0xe1e5ec:0x172028),LV_PART_MAIN);
    lv_obj_clear_flag(o,LV_OBJ_FLAG_CLICKABLE);lv_obj_add_flag(o,LV_OBJ_FLAG_EVENT_BUBBLE);return o;
}
static void arc_step(void *o,int32_t v) {lv_arc_set_value(o,v);}
void ui_animate_arc(lv_obj_t *o,float value) {
    if(!o) return;
    int v=isfinite(value)?(int)lroundf(fmaxf(0,fminf(100,value))):0;
    if(!settings.animations) {lv_anim_del(o,arc_step);lv_arc_set_value(o,v);return;}
    lv_anim_t *running=lv_anim_get(o,arc_step);
    if((running&&running->end_value==v)||(!running&&lv_arc_get_value(o)==v)) return;
    lv_anim_del(o,arc_step);
    lv_anim_t a;lv_anim_init(&a);lv_anim_set_var(&a,o);lv_anim_set_exec_cb(&a,arc_step);
    lv_anim_set_values(&a,lv_arc_get_value(o),v);lv_anim_set_time(&a,350);lv_anim_set_path_cb(&a,lv_anim_path_ease_out);lv_anim_start(&a);
}
void ui_value(lv_obj_t *o,float v,const char *unit,int precision) {
    if(!o) return;
    char s[96];if(!isfinite(v)) snprintf(s,sizeof(s),"-- %s",unit);
    else snprintf(s,sizeof(s),"%.*f%s%s",precision,v,unit[0]?" ":"",unit);
    lv_label_set_text(o,s);
}
void ui_temp(lv_obj_t *o,float v) {ui_value(o,settings.fahrenheit?v*1.8f+32:v,settings.fahrenheit?"°F":"°C",0);}
static void navigate_later(void *arg) {ui_navigate((int)(intptr_t)arg);}
static void panel_later(void *arg) {ui_panel((int)(intptr_t)arg);}
static void body_x_step(void *obj,int32_t x) {lv_obj_set_x(obj,x);}
void ui_wake(void) {
    last_touch_us=esp_timer_get_time();
    if(sleeping) {
        sleeping=false;board_brightness(settings.brightness);
        if(clock_label) {lv_obj_del(clock_label);clock_label=NULL;}
        lv_obj_clear_flag(body,LV_OBJ_FLAG_HIDDEN);
    }
}
void ui_power_countdown(int remaining) {
    if(remaining<0) {
        if(power_overlay) {lv_obj_del(power_overlay);power_overlay=NULL;power_label=NULL;}
        return;
    }
    if(remaining==10&&!power_overlay) return; /* Keep quick taps visually quiet. */
    ui_wake();
    if(!power_overlay) {
        power_overlay=lv_obj_create(root);lv_obj_remove_style_all(power_overlay);
        lv_obj_set_size(power_overlay,466,466);lv_obj_set_pos(power_overlay,0,0);
        lv_obj_clear_flag(power_overlay,LV_OBJ_FLAG_SCROLLABLE);
        lv_obj_set_style_bg_color(power_overlay,lv_color_hex(ui_theme_bg()),0);
        lv_obj_set_style_bg_opa(power_overlay,LV_OPA_COVER,0);
        ui_label(power_overlay,"POWER OFF",83,138,300,&lv_font_montserrat_24,ui_accent());
        power_label=ui_label(power_overlay,"",65,190,336,&lv_font_montserrat_18,ui_theme_fg());
        ui_label(power_overlay,"Release to cancel",65,243,336,&lv_font_montserrat_14,ui_theme_fg());
        ui_label(power_overlay,"Finish transfers and eject MoniStore",43,297,380,&lv_font_montserrat_12,0x8595a4);
    }
    char text[64];snprintf(text,sizeof(text),"Keep holding BOOT  /  %d s",remaining);
    lv_label_set_text(power_label,text);
}
static void gestures(lv_event_t *e) {
    if(power_overlay) return;
    lv_event_code_t code=lv_event_get_code(e);lv_indev_t *indev=lv_indev_get_act();if(!indev) return;
    lv_point_t pt;lv_indev_get_point(indev,&pt);int64_t now=esp_timer_get_time();
    if(code==LV_EVENT_PRESSED) {last_touch_us=now;down_x=pt.x;down_y=pt.y;dragged=false;long_pressed=false;woke_on_press=sleeping;
        ui_wake(); }
    if(code==LV_EVENT_LONG_PRESSED) {long_pressed=true;lv_async_call(navigate_later,(void *)(intptr_t)8);}
    if(code==LV_EVENT_RELEASED) {
        last_touch_us=now;int dx=pt.x-down_x,dy=pt.y-down_y;
        if(abs(dx)>55&&abs(dx)>abs(dy)) {dragged=true;lv_async_call(navigate_later,(void *)(intptr_t)((page+(dx<0?1:8))%9));}
        else if(abs(dy)>55) {dragged=true;if(panel) lv_async_call(panel_later,NULL);
            else if(page==4&&storage_next(dy<0?1:-1)) {} else lv_async_call(panel_later,(void *)(intptr_t)(dy>0?1:2)); }
        if(!dragged&&!long_pressed&&now-last_tap_us<320000) {last_tap_us=0;lv_async_call(navigate_later,NULL);}
        else if(!dragged&&!long_pressed&&!woke_on_press) {
            last_tap_us=now;
            if(page==0&&!panel&&abs(dx)<20&&abs(dy)<20&&down_x>=133&&down_x<=333&&down_y>=150&&down_y<=285) overview_cycle();
        }
    }
}
static void action(lv_event_t *e) {
    int id=(int)(intptr_t)lv_event_get_user_data(e);
    switch(id) {
        case 0: settings.theme=(settings.theme+1)%5;settings_save();lv_async_call(navigate_later,(void *)(intptr_t)page);break;
        case 1: usb_action("refresh");break;
        case 2: sleeping=true;board_brightness(0);break;
        case 3: usb_action("reconnect");break;
        case 4: settings.brightness=settings.brightness<200?230:90;board_brightness(settings.brightness);settings_save();break;
        case 5: settings.fahrenheit=!settings.fahrenheit;settings_save();break;
        case 6: settings.timeout=settings.timeout==120?0:120;settings_save();break;
        case 7: settings.animations=!settings.animations;settings_save();break;
        case 8: settings.clock_mode=(settings.clock_mode+1)%4;settings_save();break;
        case 9: settings.rate=settings.rate==1?2:1;settings_save();usb_action("interval");break;
        case 10: settings.auto_rotate=!settings.auto_rotate;settings_save();break;
    }
    if(page==8&&id!=0) lv_async_call(navigate_later,(void *)(intptr_t)8);
}
static void button(lv_obj_t *p,const char *text,int y,int id) {
    lv_obj_t *b=lv_btn_create(p);lv_obj_set_pos(b,91,y);lv_obj_set_size(b,284,44);
    lv_obj_set_style_bg_color(b,lv_color_hex(settings.theme==3?0xe5e9f0:0x111a23),0);
    lv_obj_set_style_radius(b,14,0);lv_obj_set_style_shadow_width(b,0,0);
    lv_obj_set_style_bg_color(b,lv_color_hex(ui_accent()),LV_STATE_PRESSED);
    lv_obj_add_event_cb(b,action,LV_EVENT_CLICKED,(void *)(intptr_t)id);
    lv_obj_t *label=lv_label_create(b);lv_label_set_text(label,text);lv_obj_set_style_text_font(label,&lv_font_montserrat_14,0);
    lv_obj_set_style_text_color(label,lv_color_hex(ui_theme_fg()),0);lv_obj_center(label);lv_obj_add_flag(label,LV_OBJ_FLAG_EVENT_BUBBLE);
}
void settings_build(lv_obj_t *p) {
    char s[80];snprintf(s,sizeof(s),"Theme  /  %s",themes[settings.theme]);button(p,s,117,0);
    snprintf(s,sizeof(s),"Brightness  /  %d%%",settings.brightness*100/255);button(p,s,167,4);
    snprintf(s,sizeof(s),"Temperature  /  %s",settings.fahrenheit?"Fahrenheit":"Celsius");button(p,s,217,5);
    snprintf(s,sizeof(s),"Screen timeout  /  %s",settings.timeout?"2 min":"Always on");button(p,s,267,6);
    snprintf(s,sizeof(s),"Animation  /  %s",settings.animations?"Smooth":"Off");button(p,s,317,7);
    snprintf(s,sizeof(s),"Auto rotate  /  %s",settings.auto_rotate?"On":"Off");button(p,s,367,10);
}
void settings_update(const telemetry_t *t,bool live) {}
void ui_panel(int which) {
    if(overlay) {lv_obj_del(overlay);overlay=NULL;}panel=which;if(!which) return;
    overlay=lv_obj_create(root);lv_obj_remove_style_all(overlay);lv_obj_set_size(overlay,466,466);
    lv_obj_set_style_bg_color(overlay,lv_color_hex(ui_theme_bg()),0);lv_obj_set_style_bg_opa(overlay,255,0);lv_obj_add_event_cb(overlay,gestures,LV_EVENT_ALL,NULL);
    ui_label(overlay,which==1?"NOTIFICATIONS":"QUICK ACTIONS",83,75,300,&lv_font_montserrat_18,ui_accent());
    if(which==2) {button(overlay,"Brightness",125,4);button(overlay,"Refresh telemetry",178,1);button(overlay,"Sleep display",231,2);button(overlay,"Reconnect agent",284,3);button(overlay,"Change theme",337,0);}
    else {
        telemetry_t t;model_copy(&t);bool hot=connected&&((isfinite(t.cpu_temp)&&t.cpu_temp>=85)||(isfinite(t.gpu_temp)&&t.gpu_temp>=85));
        ui_label(overlay,hot?"TEMPERATURE ALERT":connected?"ALL SYSTEMS CONNECTED":"AGENT DISCONNECTED",65,149,336,&lv_font_montserrat_18,hot?0xff5b57:ui_accent());
        ui_label(overlay,hot?"One or more sensors exceed 85°C.":connected?"Live telemetry is arriving over USB.":"Launch Monitor from the USB drive.",83,191,300,&lv_font_montserrat_14,ui_theme_fg());
        ui_label(overlay,"Firmware  " COMPANION_VERSION,90,260,286,&lv_font_montserrat_14,0x8595a4);
        ui_label(overlay,"Swipe vertically to close",90,355,286,&lv_font_montserrat_14,0x8595a4);
    }
}
void ui_navigate(int index) {
    if(index<0||index>8) return;
    ui_wake();
    panel=0;if(overlay){lv_obj_del(overlay);overlay=NULL;}
    int old=page;page=index;lv_obj_clean(body);lv_label_set_text(header,screens[page].title);
    lv_obj_set_y(header,page==0?117:75);lv_obj_set_y(connection,page==0?96:48);
    screens[page].build(body);char dots[48]={0};for(int i=0;i<9;i++) strcat(dots,i==page?"• ":". ");lv_label_set_text(pager,dots);
    if(settings.animations&&old!=page) {lv_anim_t a;lv_anim_init(&a);lv_anim_set_var(&a,body);lv_anim_set_exec_cb(&a,body_x_step);
        lv_anim_set_values(&a,old<page?65:-65,0);lv_anim_set_time(&a,250);lv_anim_set_path_cb(&a,lv_anim_path_ease_out);lv_anim_start(&a);}
    ui_refresh();
}
void ui_refresh(void) {
    lv_obj_set_style_bg_color(root,lv_color_hex(ui_theme_bg()),0);lv_obj_set_style_text_color(header,lv_color_hex(ui_theme_fg()),0);
    telemetry_t t;model_copy(&t);screens[page].update(&t,connected);
}
void ui_request_capture(void) {capture_pending=true;}
static void update(lv_timer_t *timer) {
    if(!power_overlay) board_auto_rotate();
    telemetry_t t;model_copy(&t);int64_t now=esp_timer_get_time();
    bool live=t.received_us>0&&now-t.received_us<6000000;
    if(live!=connected) {connected=live;lv_label_set_text(connection,live?"USB  •  LIVE":"AWAITING AGENT");
        lv_obj_set_style_text_color(connection,lv_color_hex(live?ui_accent():0x8595a4),0);ui_refresh();}
    if(t.seq!=last_seq) {last_seq=t.seq;screens[page].update(&t,connected);}
    if(settings.timeout&&now-last_touch_us>(int64_t)settings.timeout*1000000&&!sleeping&&!panel&&!power_overlay) {
        sleeping=true;lv_obj_add_flag(body,LV_OBJ_FLAG_HIDDEN);board_brightness(settings.brightness/5);
        clock_label=ui_label(root,"",70,175,326,&lv_font_montserrat_48,ui_accent());
    }
    if(sleeping&&clock_label) {
        char s[64];time_t ts=t.unix_time+(now-t.received_us)/1000000;struct tm tm;gmtime_r(&ts,&tm);
        if(!t.unix_time) snprintf(s,sizeof(s),"--:--");else snprintf(s,sizeof(s),"%02d:%02d",tm.tm_hour,tm.tm_min);
        lv_label_set_text(clock_label,s);lv_obj_set_pos(clock_label,70+(now/10000000)%7,175+(now/15000000)%7);
    }
    if(capture_pending) {capture_pending=false;usb_capture();}
}
void ui_init(void) {
    root=lv_scr_act();lv_obj_set_style_bg_color(root,lv_color_hex(ui_theme_bg()),0);lv_obj_clear_flag(root,LV_OBJ_FLAG_SCROLLABLE);
    lv_obj_add_event_cb(root,gestures,LV_EVENT_ALL,NULL);
    connection=ui_label(root,"AWAITING AGENT",90,48,286,&lv_font_montserrat_12,0x8595a4);
    header=ui_label(root,"SYSTEM",80,75,306,&lv_font_montserrat_18,ui_theme_fg());
    body=lv_obj_create(root);lv_obj_remove_style_all(body);lv_obj_set_size(body,466,466);lv_obj_clear_flag(body,LV_OBJ_FLAG_SCROLLABLE);
    lv_obj_add_flag(body,LV_OBJ_FLAG_EVENT_BUBBLE);
    pager=ui_label(root,"",100,430,266,&lv_font_montserrat_14,0x8595a4);
    last_touch_us=esp_timer_get_time();ui_navigate(0);lv_timer_create(update,100,NULL);
}
