#include "companion.h"
#include <math.h>
#include <stdio.h>
#include <string.h>
const screen_t screens[9]={
    {"SYSTEM",overview_build,overview_update},{"PROCESSOR",cpu_build,cpu_update},
    {"GRAPHICS",gpu_build,gpu_update},{"MEMORY",memory_build,memory_update},
    {"STORAGE",storage_build,storage_update},{"NETWORK",network_build,network_update},
    {"PROCESSES",process_build,process_update},{"AI WORKLOAD",ai_build,ai_update},
    {"SETTINGS",settings_build,settings_update}};
static lv_obj_t *value,*subtitle,*status,*detail[8],*rings[3],*chart,*window_label;
static lv_chart_series_t *series;
static lv_obj_t *gpu_rows[4][3],*gpu_hint,*gpu_touch;
static int gpu_page;
static void gpu_text(lv_obj_t *o,const char *s){if(strcmp(lv_label_get_text(o),s))lv_label_set_text(o,s);}
static void gpu_number(char *dst,size_t size,float n,const char *unit,int precision);
static int window_seconds=60,metric,drive_index,sort_mode;
int overview_mode(void) {return settings.center_mode;}
void overview_select(int mode) {if(mode>=0&&mode<6) settings.center_mode=mode;}
void overview_cycle(void) {settings.center_mode=(settings.center_mode+1)%6;settings_save();ui_refresh();}
static lv_obj_t *label(lv_obj_t *p,const char *s,int y,int w,const lv_font_t *font,uint32_t c) {return ui_label(p,s,(466-w)/2,y,w,font,c);}
static void pair(lv_obj_t *p,int row,const char *a,const char *b) {
    detail[row*2]=ui_label(p,a,73,330+row*36,160,&lv_font_montserrat_14,0x8595a4);
    detail[row*2+1]=ui_label(p,b,233,330+row*36,160,&lv_font_montserrat_14,0x8595a4);
}
static void choose_window(lv_event_t *e) {
    window_seconds=window_seconds==60?300:window_seconds==300?1800:60;
    lv_label_set_text(window_label,window_seconds==60?"60 SEC  /  TAP":window_seconds==300?"5 MIN  /  TAP":"30 MIN  /  TAP");ui_refresh();
}
static void graph(lv_obj_t *p,int kind,uint32_t c) {
    metric=kind;chart=lv_chart_create(p);lv_obj_set_pos(chart,83,250);lv_obj_set_size(chart,300,70);
    lv_obj_set_style_bg_opa(chart,0,0);lv_obj_set_style_border_width(chart,0,0);lv_obj_set_style_pad_all(chart,0,0);
    lv_obj_set_style_line_color(chart,lv_color_hex(0x1d2833),LV_PART_MAIN);lv_obj_set_style_line_width(chart,2,LV_PART_ITEMS);
    lv_obj_set_style_size(chart,0,LV_PART_INDICATOR);lv_chart_set_div_line_count(chart,3,0);lv_chart_set_point_count(chart,60);
    lv_chart_set_range(chart,LV_CHART_AXIS_PRIMARY_Y,0,100);series=lv_chart_add_series(chart,lv_color_hex(c),LV_CHART_AXIS_PRIMARY_Y);
    window_label=label(p,window_seconds==60?"60 SEC  /  TAP":window_seconds==300?"5 MIN  /  TAP":"30 MIN  /  TAP",225,220,&lv_font_montserrat_12,0x8595a4);
    lv_obj_set_height(window_label,26);lv_obj_add_flag(window_label,LV_OBJ_FLAG_CLICKABLE);lv_obj_add_event_cb(window_label,choose_window,LV_EVENT_CLICKED,NULL);
}
static void update_graph(bool live) {
    if(!chart) return;
    float v[60];model_history(v,metric,window_seconds,60);float max=100;
    if(metric==3) {max=1;for(int i=0;i<60;i++) if(isfinite(v[i])&&v[i]>max) max=v[i];max=ceilf(max*1.2f);}
    lv_chart_set_range(chart,LV_CHART_AXIS_PRIMARY_Y,0,(int)max);
    for(int i=0;i<60;i++) series->y_points[i]=live&&isfinite(v[i])?(lv_coord_t)fminf(32000,v[i]):LV_CHART_POINT_NONE;
    lv_chart_refresh(chart);
}
static void metric_build(lv_obj_t *p,int kind,const char *unit) {
    chart=NULL;subtitle=label(p,"",110,310,&lv_font_montserrat_14,0x8595a4);
    value=label(p,"--",141,320,&lv_font_montserrat_48,ui_theme_fg());status=label(p,unit,198,310,&lv_font_montserrat_14,ui_accent());
    graph(p,kind,ui_accent());pair(p,0,"--","--");pair(p,1,"--","--");pair(p,2,"--","--");
}
static void formatted(lv_obj_t *o,const char *name,float n,const char *unit,int precision) {
    char s[96];if(isfinite(n)) snprintf(s,sizeof(s),"%s %.*f%s",name,precision,n,unit);else snprintf(s,sizeof(s),"%s --",name);lv_label_set_text(o,s);
}
void overview_build(lv_obj_t *p) {
    chart=NULL;rings[0]=ui_ring(p,414,26,12,ui_accent());rings[1]=ui_ring(p,370,48,12,0xc581ff);rings[2]=ui_ring(p,326,70,12,0xb9f36b);
    value=label(p,"-- °C",161,235,&lv_font_montserrat_48,ui_theme_fg());status=label(p,"READY TO CONNECT",221,220,&lv_font_montserrat_14,ui_accent());
    subtitle=label(p,"Launch Monitor on your computer",250,260,&lv_font_montserrat_12,0x8595a4);
    detail[0]=ui_label(p,"CPU --",123,354,78,&lv_font_montserrat_14,ui_accent());
    detail[1]=ui_label(p,"GPU --",201,354,78,&lv_font_montserrat_14,0xc581ff);
    detail[2]=ui_label(p,"RAM --",279,354,78,&lv_font_montserrat_14,0xb9f36b);
    detail[3]=label(p,"SWIPE TO EXPLORE",383,300,&lv_font_montserrat_12,0x8595a4);
}
void overview_update(const telemetry_t *t,bool live) {
    int center_mode=overview_mode();
    float selected_gpu_temp=NAN;
    for(int i=0;i<t->gpu_count;i++) if(isfinite(t->gpus[i].temp)&&(!isfinite(selected_gpu_temp)||t->gpus[i].temp>selected_gpu_temp)) selected_gpu_temp=t->gpus[i].temp;
    if(!t->gpu_count&&!t->gpu_detected) selected_gpu_temp=t->gpu_temp;
    ui_animate_arc(rings[0],live?t->cpu_load:NAN);ui_animate_arc(rings[1],live?t->gpu_load:NAN);
    ui_animate_arc(rings[2],live&&t->ram_total>0?t->ram_used/t->ram_total*100:NAN);
    lv_obj_set_style_text_font(value,center_mode>=4?&lv_font_montserrat_24:&lv_font_montserrat_48,0);
    const char *caption="TEMPERATURE UNAVAILABLE";
    switch(center_mode) {
        case 0: ui_temp(value,live?(isfinite(t->cpu_temp)?t->cpu_temp:selected_gpu_temp):NAN);
            if(!isfinite(t->cpu_temp)&&isfinite(t->thermal_temp)&&live){ui_temp(value,t->thermal_temp);caption="SYSTEM THERMAL ZONE";}
            else {caption=isfinite(t->cpu_temp)?"CPU TEMPERATURE":isfinite(selected_gpu_temp)?"GPU TEMPERATURE":"TEMPERATURE UNAVAILABLE";}
            break;
        case 1: ui_value(value,live?t->cpu_load:NAN,"%",0);caption="PROCESSOR LOAD";break;
        case 2: ui_value(value,live?t->gpu_load:NAN,"%",0);caption="GRAPHICS LOAD";break;
        case 3: ui_value(value,live&&t->ram_total>0?t->ram_used/t->ram_total*100:NAN,"%",0);caption="MEMORY USAGE";break;
        case 4: {char down[24],up[24],s[64];
            if(live&&isfinite(t->net_down)) snprintf(down,sizeof(down),"%.1f",t->net_down);else strcpy(down,"--");
            if(live&&isfinite(t->net_up)) snprintf(up,sizeof(up),"%.1f",t->net_up);else strcpy(up,"--");
            snprintf(s,sizeof(s),"DL %s Mbps\nUL %s Mbps",down,up);lv_label_set_text(value,s);caption="DOWNLOAD / UPLOAD";break;}
        case 5: {char cpu[24],gpu[24],s[80];const char *unit=settings.fahrenheit?"°F":"°C";
            if(live&&isfinite(t->cpu_temp)) snprintf(cpu,sizeof(cpu),"%.0f%s",settings.fahrenheit?t->cpu_temp*1.8f+32:t->cpu_temp,unit);else snprintf(cpu,sizeof(cpu),"--%s",unit);
            if(live&&isfinite(t->gpu_temp)) snprintf(gpu,sizeof(gpu),"%.0f%s",settings.fahrenheit?t->gpu_temp*1.8f+32:t->gpu_temp,unit);else snprintf(gpu,sizeof(gpu),"--%s",unit);
            snprintf(s,sizeof(s),"CPU  %s\nGPU  %s",cpu,gpu);lv_label_set_text(value,s);caption="CPU / GPU TEMPERATURES";break;}
    }
    lv_obj_set_y(value,161);lv_obj_set_y(status,221);lv_obj_set_y(subtitle,250);
    if(t->gpu_count>1&&(center_mode==2||center_mode==5)){
        char multi[240]="",line[48];int shown=t->gpu_count>4?4:t->gpu_count;
        const char *unit=settings.fahrenheit?"°F":"°C";
        if(center_mode==5){char cpu[24];if(live&&isfinite(t->cpu_temp))snprintf(cpu,sizeof(cpu),"%.0f%s",settings.fahrenheit?t->cpu_temp*1.8f+32:t->cpu_temp,unit);else snprintf(cpu,sizeof(cpu),"--%s",unit);snprintf(multi,sizeof(multi),"CPU  %s",cpu);}
        for(int i=0;i<shown;i++){
            float n=center_mode==2?t->gpus[i].load:t->gpus[i].temp;
            if(center_mode==5&&settings.fahrenheit&&isfinite(n))n=n*1.8f+32;
            if(live&&isfinite(n))snprintf(line,sizeof(line),"%sGPU %d  %.0f%s",multi[0]?"\n":"",t->gpus[i].index,n,center_mode==2?"%":unit);
            else snprintf(line,sizeof(line),"%sGPU %d  --",multi[0]?"\n":"",t->gpus[i].index);
            strncat(multi,line,sizeof(multi)-strlen(multi)-1);
        }
        lv_label_set_text(value,multi);lv_obj_set_style_text_font(value,shown>=3?&lv_font_montserrat_18:&lv_font_montserrat_24,0);
        lv_obj_set_y(value,shown>=3?139:151);lv_obj_set_y(status,shown>=3?263:246);lv_obj_set_y(subtitle,shown>=3?287:270);
    }
    if(t->gpu_count==1&&(center_mode==2||center_mode==5)) {
        char text[128],gpu[32],cpu[32];const gpu_t *g=&t->gpus[0];
        if(center_mode==2) {lv_obj_set_style_text_font(value,&lv_font_montserrat_24,0);gpu_number(gpu,sizeof(gpu),live?g->load:NAN,"%",0);snprintf(text,sizeof(text),"GPU%d  %s",g->index,gpu);}
        else {const char *unit=settings.fahrenheit?"°F":"°C";float n=g->temp,c=t->cpu_temp;if(settings.fahrenheit){n=n*1.8f+32;c=c*1.8f+32;}
            gpu_number(gpu,sizeof(gpu),live?n:NAN,unit,0);gpu_number(cpu,sizeof(cpu),live?c:NAN,unit,0);snprintf(text,sizeof(text),"CPU  %s\nGPU%d  %s",cpu,g->index,gpu);}
        lv_label_set_text(value,text);
    }
    if(!t->gpu_count&&t->gpu_detected&&(center_mode==2||center_mode==5)) {
        if(center_mode==2) {ui_value(value,live?t->gpu_load:NAN,"%",0);caption="NO GPUS SELECTED";}
        else {ui_temp(value,live?t->cpu_temp:NAN);caption="CPU TEMPERATURE";}
    }
    bool hot=(isfinite(t->cpu_temp)&&t->cpu_temp>=85)||(isfinite(t->gpu_temp)&&t->gpu_temp>=85);
    lv_label_set_text(status,live?caption:"READY TO CONNECT");
    lv_obj_set_style_text_color(status,lv_color_hex(hot&&live?0xff5b57:ui_accent()),0);
    lv_label_set_text(subtitle,live?"TAP CENTRE TO CHANGE":"Launch Monitor on your computer");
    formatted(detail[0],"CPU",live?t->cpu_load:NAN,"%",0);formatted(detail[1],"GPU",live?t->gpu_load:NAN,"%",0);
    formatted(detail[2],"RAM",live&&t->ram_total>0?t->ram_used/t->ram_total*100:NAN,"%",0);
    char s[100];if(live) {snprintf(s,sizeof(s),"USB CONNECTED  /  %s",t->gpu_name[0]?t->gpu_name:"SYSTEM TELEMETRY");lv_label_set_text(detail[3],s);}else lv_label_set_text(detail[3],"SWIPE TO EXPLORE");
}
void cpu_build(lv_obj_t *p) {metric_build(p,0,"PACKAGE TEMPERATURE");}
void cpu_update(const telemetry_t *t,bool live) {
    lv_label_set_text(subtitle,t->cpu_name);
    if(live&&!isfinite(t->cpu_temp)&&!isfinite(t->thermal_temp)) {ui_value(value,t->cpu_load,"%",0);lv_label_set_text(status,"LOAD / TEMPERATURE UNAVAILABLE");}
    else {ui_temp(value,live?(isfinite(t->cpu_temp)?t->cpu_temp:t->thermal_temp):NAN);lv_label_set_text(status,isfinite(t->cpu_temp)?"CPU TEMPERATURE":isfinite(t->thermal_temp)?"THERMAL ZONE (NOT CPU DIE)":"CPU TEMPERATURE UNAVAILABLE");}
    update_graph(live);
    formatted(detail[0],"LOAD",live?t->cpu_load:NAN,"%",0);formatted(detail[1],"POWER",live?t->cpu_power:NAN," W",0);
    formatted(detail[2],"CLOCK",live?t->cpu_clock/1000:NAN," GHz",2);formatted(detail[3],"VOLT",live?t->cpu_voltage:NAN," V",2);
    formatted(detail[4],"CORES",live&&t->core_count?t->core_count:NAN,"",0);lv_label_set_text(detail[5],"Tap graph time range");
}
static void gpu_next(lv_event_t *e){(void)e;if(ui_page()!=2)return;telemetry_t t;model_copy(&t);if(t.gpu_count>4){gpu_page++;ui_refresh();}}
static void gpu_visibility(lv_obj_t *o,bool visible){if(!o)return;if(visible)lv_obj_clear_flag(o,LV_OBJ_FLAG_HIDDEN);else lv_obj_add_flag(o,LV_OBJ_FLAG_HIDDEN);}
static void gpu_number(char *dst,size_t size,float n,const char *unit,int precision){if(isfinite(n))snprintf(dst,size,"%.*f%s",precision,n,unit);else snprintf(dst,size,"--");}
void gpu_build(lv_obj_t *p){
    chart=NULL;gpu_page=0;
    gpu_touch=lv_obj_create(p);lv_obj_remove_style_all(gpu_touch);lv_obj_set_pos(gpu_touch,58,112);lv_obj_set_size(gpu_touch,350,294);
    lv_obj_clear_flag(gpu_touch,LV_OBJ_FLAG_SCROLLABLE);lv_obj_add_event_cb(gpu_touch,gpu_next,LV_EVENT_CLICKED,NULL);
    for(int i=0;i<4;i++) for(int j=0;j<3;j++) {
        gpu_rows[i][j]=label(p,"",120,326,&lv_font_montserrat_12,j==0?ui_accent():ui_theme_fg());
        lv_label_set_long_mode(gpu_rows[i][j],j==0?LV_LABEL_LONG_DOT:LV_LABEL_LONG_CLIP);
    }
    gpu_hint=label(p,"",408,310,&lv_font_montserrat_12,0x8595a4);
}
void gpu_update(const telemetry_t *t,bool live){
    gpu_t legacy={0};int total=t->gpu_count;
    if(!total&&!t->gpu_detected&&(t->gpu_name[0]||isfinite(t->gpu_temp))) {
        snprintf(legacy.name,sizeof(legacy.name),"%s",t->gpu_name);
        legacy.temp=t->gpu_temp;legacy.load=t->gpu_load;legacy.power=t->gpu_power;
        legacy.vram=t->gpu_vram;legacy.vram_total=t->gpu_vram_total;total=1;
    }
    gpu_visibility(gpu_touch,total>4);gpu_visibility(gpu_hint,true);
    lv_obj_set_y(gpu_hint,total?408:215);
    if(!total) {for(int i=0;i<4;i++)for(int j=0;j<3;j++)gpu_visibility(gpu_rows[i][j],false);lv_label_set_text(gpu_hint,t->gpu_detected?"NO GPUS SELECTED / USE DASHBOARD":"NO GPU DETECTED");return;}
    int pages=(total+3)/4,start=(gpu_page%pages)*4,count=total-start;
    if(count>4) count=4;
    for(int i=0;i<4;i++)for(int j=0;j<3;j++)gpu_visibility(gpu_rows[i][j],i<count);
    int row_height=count==1?200:count==2?132:count==3?96:72;
    int top=count==1?150:count==2?137:116;
    for(int i=0;i<count;i++) {
        const gpu_t *g=t->gpu_count?&t->gpus[start+i]:&legacy;
        char text[160],temp[24],load[24],power[24],used[24],capacity[24];
        float n=g->temp;const char *unit=settings.fahrenheit?"°F":"°C";
        if(settings.fahrenheit) n=n*1.8f+32;
        gpu_number(temp,sizeof(temp),live?n:NAN,unit,0);gpu_number(load,sizeof(load),live?g->load:NAN,"%",0);
        gpu_number(power,sizeof(power),live?g->power:NAN,"W",0);gpu_number(used,sizeof(used),live?g->vram:NAN,"",1);
        gpu_number(capacity,sizeof(capacity),live?g->vram_total:NAN,"GB",1);
        int y=top+i*row_height;
        for(int j=0;j<3;j++) {
            lv_obj_set_y(gpu_rows[i][j],y+(count==1?j*42:count==2?j*30:count==3?j*24:(j==0?0:j==1?18:44)));
            const lv_font_t *font=count==1?(j==1?&lv_font_montserrat_24:&lv_font_montserrat_18):count==2?(j==1?&lv_font_montserrat_24:&lv_font_montserrat_14):j==1?&lv_font_montserrat_18:&lv_font_montserrat_12;
            if(lv_obj_get_style_text_font(gpu_rows[i][j],0)!=font)lv_obj_set_style_text_font(gpu_rows[i][j],font,0);
            if(j==0)lv_obj_set_height(gpu_rows[i][j],lv_font_get_line_height(font));
        }
        snprintf(text,sizeof(text),"GPU%d  %s",g->index,g->name);gpu_text(gpu_rows[i][0],text);
        snprintf(text,sizeof(text),"%s   LOAD %s",temp,load);gpu_text(gpu_rows[i][1],text);
        snprintf(text,sizeof(text),"VRAM %s/%s   POWER %s",used,capacity,power);gpu_text(gpu_rows[i][2],text);
    }
    char hint[96];snprintf(hint,sizeof(hint),"%d-%d / %d SHOWN%s",t->gpu_offset+start+1,t->gpu_offset+start+count,t->gpu_total?t->gpu_total:total,pages>1?"  /  TAP FOR MORE":"");lv_label_set_text(gpu_hint,hint);
}

void memory_build(lv_obj_t *p) {metric_build(p,2,"PHYSICAL MEMORY USED");}
void memory_update(const telemetry_t *t,bool live) {
    lv_label_set_text(subtitle,"SYSTEM MEMORY");ui_value(value,live?t->ram_used:NAN,"GB",1);update_graph(live);
    formatted(detail[0],"TOTAL",live?t->ram_total:NAN," GB",1);formatted(detail[1],"FREE",live?t->ram_available:NAN," GB",1);
    formatted(detail[2],"CACHED",live?t->ram_cached:NAN," GB",1);formatted(detail[3],"SWAP",live?t->swap_used:NAN," GB",1);
    formatted(detail[4],"USED",live&&t->ram_total>0?t->ram_used/t->ram_total*100:NAN,"%",0);formatted(detail[5],"COMMIT",live?t->ram_commit:NAN," GB",1);
}
void storage_build(lv_obj_t *p) {metric_build(p,2,"CAPACITY USED");lv_obj_add_flag(chart,LV_OBJ_FLAG_HIDDEN);lv_obj_add_flag(window_label,LV_OBJ_FLAG_HIDDEN);}
bool storage_next(int delta) {telemetry_t t;model_copy(&t);if(t.drive_count<2) return false;drive_index=(drive_index+t.drive_count+delta)%t.drive_count;storage_update(&t,ui_is_connected());return true;}
void storage_update(const telemetry_t *t,bool live) {
    if(!t->drive_count) {lv_label_set_text(subtitle,"No drive data available");ui_value(value,NAN,"%",0);return;}
    if(drive_index>=t->drive_count) drive_index=0;
    const drive_t *d=&t->drives[drive_index];
    lv_label_set_text(subtitle,d->name);ui_value(value,live&&d->total>0?d->used/d->total*100:NAN,"%",0);
    formatted(detail[0],"USED",live?d->used:NAN," GB",0);formatted(detail[1],"TOTAL",live?d->total:NAN," GB",0);
    formatted(detail[2],"READ",live?d->read:NAN," MB/s",1);formatted(detail[3],"WRITE",live?d->write:NAN," MB/s",1);
    formatted(detail[4],"TEMP",live?d->temperature:NAN,"°C",0);formatted(detail[5],"HEALTH",live?d->health:NAN,"%",0);
    char s[64];snprintf(s,sizeof(s),"DRIVE %d / %d  /  SWIPE VERTICALLY",drive_index+1,t->drive_count);lv_label_set_text(status,s);
}
void network_build(lv_obj_t *p) {metric_build(p,3,"DOWNLOAD");}
void network_update(const telemetry_t *t,bool live) {
    lv_label_set_text(subtitle,t->net_adapter);ui_value(value,live?t->net_down:NAN,"Mbps",1);update_graph(live);
    formatted(detail[0],"UP",live?t->net_up:NAN," Mbps",1);formatted(detail[1],"PING",live?t->net_ping:NAN," ms",0);
    formatted(detail[2],"LOSS",live?t->net_loss:NAN,"%",1);formatted(detail[3],"LINK",live?t->net_utilization:NAN,"%",0);
    lv_label_set_text(detail[4],"Actual traffic");lv_label_set_text(detail[5],"1 Mbps = 0.125 MB/s");
}
static void sort_process(lv_event_t *e) {sort_mode=(sort_mode+1)%2;lv_label_set_text(status,sort_mode?"SORT: MEMORY  /  TAP":"SORT: CPU  /  TAP");ui_refresh();}
void process_build(lv_obj_t *p) {
    chart=NULL;status=label(p,sort_mode?"SORT: MEMORY  /  TAP":"SORT: CPU  /  TAP",120,286,&lv_font_montserrat_14,ui_accent());
    lv_obj_set_height(status,32);lv_obj_add_flag(status,LV_OBJ_FLAG_CLICKABLE);lv_obj_add_event_cb(status,sort_process,LV_EVENT_CLICKED,NULL);
    for(int i=0;i<5;i++) detail[i]=label(p,"--",172+i*43,290,&lv_font_montserrat_18,ui_theme_fg());
}
void process_update(const telemetry_t *t,bool live) {
    int ids[PROCESS_MAX];for(int i=0;i<t->process_count;i++) ids[i]=i;
    for(int i=0;i<t->process_count;i++) for(int j=i+1;j<t->process_count;j++) {
        const process_t *a=&t->processes[ids[i]],*b=&t->processes[ids[j]];
        if((sort_mode?b->ram:b->cpu)>(sort_mode?a->ram:a->cpu)) {int k=ids[i];ids[i]=ids[j];ids[j]=k;}}
    for(int i=0;i<5;i++) {char s[100];if(live&&i<t->process_count) {const process_t *p=&t->processes[ids[i]];
        snprintf(s,sizeof(s),"%.18s   %.1f %s",p->name,sort_mode?p->ram:p->cpu,sort_mode?"GB":"%");} else snprintf(s,sizeof(s),"--");lv_label_set_text(detail[i],s);}
}
void ai_build(lv_obj_t *p) {
    chart=NULL;subtitle=label(p,"LOCAL AI",112,310,&lv_font_montserrat_14,0x8595a4);
    value=label(p,"NO SOURCE",171,330,&lv_font_montserrat_24,ui_accent());status=label(p,"No local AI source detected",223,300,&lv_font_montserrat_18,ui_theme_fg());
    lv_label_set_long_mode(status,LV_LABEL_LONG_DOT);lv_obj_set_height(status,26);
    pair(p,0,"CPU --","RAM --");pair(p,1,"KV CACHE --","QUEUE --");
}
void ai_update(const telemetry_t *t,bool live) {
    lv_label_set_text(subtitle,live&&t->ai_provider[0]?t->ai_provider:"LOCAL AI");
    const char *state=live?(t->ai_state[0]?t->ai_state:"NO SOURCE"):"OFFLINE";
    lv_label_set_text(value,!strcmp(state,"WORKER")?"ENGINE RUNNING":state);
    lv_label_set_text(status,!live?"Connect the monitoring client":t->ai_model[0]?t->ai_model:t->ai_note[0]?t->ai_note:"No local AI source detected");
    if(live&&isfinite(t->ai_tps)) formatted(detail[0],"TOKENS/S",t->ai_tps,"",1);
    else formatted(detail[0],"CPU",live?t->ai_cpu:NAN,"%",1);
    if(live&&isfinite(t->ai_vram)) formatted(detail[1],"VRAM",t->ai_vram," GB",1);
    else formatted(detail[1],"RAM",live?t->ai_ram:NAN," GB",1);
    formatted(detail[2],"KV CACHE",live?t->ai_context:NAN,"%",0);formatted(detail[3],"QUEUE",live?t->ai_queue:NAN,"",0);
}
