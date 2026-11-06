//go:build linux && gtk

#include "gtk_linux.h"
struct ShomerUI {
    GtkWidget *window, *title, *subtitle, *body, *log, *footer, *eyebrow;
    GtkWidget *sidebar[6], *cards, *values[4], *options, *directory;
    GtkWidget *backup, *userdata, *erase, *next, *back, *progress, *brom;
    int events[32], read, write, rendering;
};
static void action(GtkWidget *widget, gpointer data) {
    ShomerUI *u=data;
    if (!u->rendering && u->write-u->read<32) u->events[(u->write++)%32]=GPOINTER_TO_INT(g_object_get_data(G_OBJECT(widget),"action"));
}
static gboolean closing(GtkWidget *widget, GdkEvent *event, gpointer data) {
    ShomerUI *u=data;
    if(u->write-u->read<32) u->events[(u->write++)%32]=3;
    return TRUE;
}
static GtkWidget *label(const char *s, const char *class) {
    GtkWidget *w=gtk_label_new(s); gtk_label_set_xalign(GTK_LABEL(w),0);
    gtk_label_set_line_wrap(GTK_LABEL(w),TRUE);
    if(class)gtk_style_context_add_class(gtk_widget_get_style_context(w),class);
    return w;
}
static GtkWidget *button(ShomerUI *u, const char *text, int id) {
    GtkWidget *w=gtk_button_new_with_label(text);
    g_object_set_data(G_OBJECT(w),"action",GINT_TO_POINTER(id));
    g_signal_connect(w,"clicked",G_CALLBACK(action),u);return w;
}
static GtkWidget *toggle(ShomerUI *u, const char *text, int id) {
    GtkWidget *w=gtk_check_button_new_with_label(text);
    g_object_set_data(G_OBJECT(w),"action",GINT_TO_POINTER(id));
    g_signal_connect(w,"toggled",G_CALLBACK(action),u);return w;
}
static GtkWidget *textview(GtkWidget **view, gboolean expand) {
    GtkWidget *scroll=gtk_scrolled_window_new(NULL,NULL);*view=gtk_text_view_new();
    gtk_text_view_set_editable(GTK_TEXT_VIEW(*view),FALSE);
    gtk_text_view_set_cursor_visible(GTK_TEXT_VIEW(*view),FALSE);
    gtk_text_view_set_wrap_mode(GTK_TEXT_VIEW(*view),GTK_WRAP_WORD_CHAR);
    gtk_text_view_set_left_margin(GTK_TEXT_VIEW(*view),16);gtk_text_view_set_right_margin(GTK_TEXT_VIEW(*view),16);
    gtk_text_view_set_top_margin(GTK_TEXT_VIEW(*view),12);gtk_text_view_set_bottom_margin(GTK_TEXT_VIEW(*view),12);
    gtk_container_add(GTK_CONTAINER(scroll),*view);gtk_widget_set_vexpand(scroll,expand);return scroll;
}
ShomerUI *shomer_ui_new(const char *version) {
    if(!gtk_init_check(NULL,NULL))return NULL;
    ShomerUI *u=g_new0(ShomerUI,1);
    GtkCssProvider *css=gtk_css_provider_new();
    gtk_css_provider_load_from_data(css,
      "window,box,grid {background:#f8f6ef;color:#163635;}"
      "label {font-family:Sans;font-size:14px;} .brand {font-size:22px;font-weight:bold;}"
      ".title {font-family:Monospace;font-size:27px;font-weight:bold;} .eyebrow {font-family:Monospace;font-size:11px;color:#506766;}"
      ".muted {color:#586b6a;} .stage {padding:16px 12px;border-radius:6px;}"
      ".active {background:#dce9df;color:#076c68;font-weight:bold;} .card {border:1px solid #cbd5cc;border-radius:7px;padding:18px;}"
      ".card-title {font-family:Monospace;font-size:11px;font-weight:bold;color:#33605d;}"
      "textview text,textview {background:#fffdf7;color:#163635;font-family:Sans;font-size:14px;}"
      "button {padding:10px 14px;border-radius:5px;} .primary {background:#258b85;color:white;font-weight:bold;}"
      "entry {padding:8px;} progressbar progress {background:#258b85;}"
      "checkbutton {padding:8px 0;}", -1,NULL);
    gtk_style_context_add_provider_for_screen(gdk_screen_get_default(),GTK_STYLE_PROVIDER(css),GTK_STYLE_PROVIDER_PRIORITY_APPLICATION);
    g_object_unref(css);
    u->window=gtk_window_new(GTK_WINDOW_TOPLEVEL);
    gtk_window_set_title(GTK_WINDOW(u->window),"Shomer Flip 2 Workbench");
    GdkRectangle area={0,0,1280,800};GdkDisplay *display=gdk_display_get_default();GdkMonitor *monitor=gdk_display_get_primary_monitor(display);if(!monitor)monitor=gdk_display_get_monitor_at_point(display,0,0);if(monitor)gdk_monitor_get_workarea(monitor,&area);
    gtk_window_set_default_size(GTK_WINDOW(u->window),MIN(1024,area.width-24),MIN(740,area.height-48));
    gtk_widget_set_size_request(u->window,MIN(880,area.width-24),MIN(640,area.height-48));
    g_signal_connect(u->window,"delete-event",G_CALLBACK(closing),u);
    GtkWidget *outer=gtk_box_new(GTK_ORIENTATION_VERTICAL,0),*row=gtk_box_new(GTK_ORIENTATION_HORIZONTAL,0);
    gtk_container_add(GTK_CONTAINER(u->window),outer);gtk_box_pack_start(GTK_BOX(outer),row,TRUE,TRUE,0);
    GtkWidget *side=gtk_box_new(GTK_ORIENTATION_VERTICAL,7);gtk_widget_set_size_request(side,200,-1);
    gtk_container_set_border_width(GTK_CONTAINER(side),20);gtk_box_pack_start(GTK_BOX(row),side,FALSE,FALSE,0);
    gtk_box_pack_start(GTK_BOX(side),label("SHOMER","brand"),FALSE,FALSE,8);
    gtk_box_pack_start(GTK_BOX(side),label("Flip 2 Workbench","muted"),FALSE,FALSE,12);
    const char *stages[]={"1   Check images","2   Identify phone","3   Back up","4   Review","5   Install","6   Results"};
    for(int i=0;i<6;i++){u->sidebar[i]=label(stages[i],"stage");gtk_box_pack_start(GTK_BOX(side),u->sidebar[i],FALSE,FALSE,2);}
    gtk_box_pack_end(GTK_BOX(side),label(version,"muted"),FALSE,FALSE,8);
    gtk_box_pack_start(GTK_BOX(row),gtk_separator_new(GTK_ORIENTATION_VERTICAL),FALSE,FALSE,0);
    GtkWidget *main=gtk_box_new(GTK_ORIENTATION_VERTICAL,14);gtk_container_set_border_width(GTK_CONTAINER(main),26);
    gtk_box_pack_start(GTK_BOX(row),main,TRUE,TRUE,0);
    u->eyebrow=label("STEP 1 OF 6","eyebrow");u->title=label("Check your images","title");u->subtitle=label("","muted");
    gtk_box_pack_start(GTK_BOX(main),u->eyebrow,FALSE,FALSE,0);gtk_box_pack_start(GTK_BOX(main),u->title,FALSE,FALSE,0);gtk_box_pack_start(GTK_BOX(main),u->subtitle,FALSE,FALSE,0);
    u->cards=gtk_grid_new();gtk_grid_set_row_spacing(GTK_GRID(u->cards),12);gtk_grid_set_column_spacing(GTK_GRID(u->cards),12);gtk_grid_set_column_homogeneous(GTK_GRID(u->cards),TRUE);
    const char *titles[]={"DETECTED IDENTITY","SELECTED IDENTITY","CONNECTION MODE","UNLOCK STATE"};
    for(int i=0;i<4;i++){GtkWidget *card=gtk_box_new(GTK_ORIENTATION_VERTICAL,10);gtk_style_context_add_class(gtk_widget_get_style_context(card),"card");gtk_box_pack_start(GTK_BOX(card),label(titles[i],"card-title"),FALSE,FALSE,0);u->values[i]=label("Not verified",NULL);gtk_box_pack_start(GTK_BOX(card),u->values[i],TRUE,TRUE,0);gtk_grid_attach(GTK_GRID(u->cards),card,i%2,i/2,1,1);}
    gtk_box_pack_start(GTK_BOX(main),u->cards,FALSE,FALSE,0);
    gtk_box_pack_start(GTK_BOX(main),textview(&u->body,TRUE),TRUE,TRUE,0);
    u->options=gtk_box_new(GTK_ORIENTATION_VERTICAL,5);gtk_box_pack_start(GTK_BOX(main),u->options,FALSE,FALSE,0);
    u->backup=toggle(u,"Recovery backup first (fallback when BROM is unavailable)",5);gtk_box_pack_start(GTK_BOX(u->options),u->backup,FALSE,FALSE,0);
    GtkWidget *dir=gtk_box_new(GTK_ORIENTATION_HORIZONTAL,8);u->directory=gtk_entry_new();gtk_box_pack_start(GTK_BOX(dir),u->directory,TRUE,TRUE,0);gtk_box_pack_start(GTK_BOX(dir),button(u,"Choose folder…",4),FALSE,FALSE,0);gtk_box_pack_start(GTK_BOX(u->options),dir,FALSE,FALSE,0);
    g_object_set_data(G_OBJECT(u->directory),"action",GINT_TO_POINTER(9));g_signal_connect(u->directory,"changed",G_CALLBACK(action),u);
    u->userdata=toggle(u,"Include encrypted phone data (already unlocked phones only)",6);u->erase=toggle(u,"Erase phone data after installation",7);
    gtk_box_pack_start(GTK_BOX(u->options),u->userdata,FALSE,FALSE,0);gtk_box_pack_start(GTK_BOX(u->options),u->erase,FALSE,FALSE,0);
    u->progress=gtk_progress_bar_new();gtk_progress_bar_set_show_text(GTK_PROGRESS_BAR(u->progress),TRUE);gtk_box_pack_start(GTK_BOX(main),u->progress,FALSE,FALSE,0);
    GtkWidget *expander=gtk_expander_new("Activity log");GtkWidget *logScroll=textview(&u->log,FALSE);gtk_widget_set_size_request(logScroll,-1,130);gtk_container_add(GTK_CONTAINER(expander),logScroll);gtk_box_pack_start(GTK_BOX(main),expander,FALSE,FALSE,0);
    GtkWidget *bottom=gtk_box_new(GTK_ORIENTATION_HORIZONTAL,10);gtk_container_set_border_width(GTK_CONTAINER(bottom),16);gtk_box_pack_end(GTK_BOX(outer),bottom,FALSE,FALSE,0);
    u->footer=label("Works offline. Keep your backups private.","muted");gtk_box_pack_start(GTK_BOX(bottom),u->footer,TRUE,TRUE,0);
    u->brom=button(u,"BROM backup…",8);gtk_box_pack_start(GTK_BOX(bottom),u->brom,FALSE,FALSE,0);
    gtk_box_pack_start(GTK_BOX(bottom),button(u,"Close",3),FALSE,FALSE,0);u->back=button(u,"Back",2);gtk_box_pack_start(GTK_BOX(bottom),u->back,FALSE,FALSE,0);
    u->next=button(u,"Continue",1);gtk_style_context_add_class(gtk_widget_get_style_context(u->next),"primary");gtk_widget_set_can_default(u->next,TRUE);gtk_window_set_default(GTK_WINDOW(u->window),u->next);gtk_box_pack_start(GTK_BOX(bottom),u->next,FALSE,FALSE,0);
    gtk_widget_show_all(u->window);return u;
}
void shomer_ui_destroy(ShomerUI *u){gtk_widget_destroy(u->window);g_free(u);}
void shomer_ui_pump(void){while(gtk_events_pending())gtk_main_iteration_do(FALSE);}
int shomer_ui_event(ShomerUI *u){return u->read==u->write?0:u->events[(u->read++)%32];}
void shomer_ui_text(ShomerUI *u,int field,const char *text){
    if(field==3||field==4){GtkWidget *v=field==3?u->body:u->log;gtk_text_buffer_set_text(gtk_text_view_get_buffer(GTK_TEXT_VIEW(v)),text,-1);return;}
    GtkWidget *w=field==0?u->eyebrow:field==1?u->title:field==2?u->subtitle:field==5?u->footer:u->values[field-6];gtk_label_set_text(GTK_LABEL(w),text);
}
void shomer_ui_state(ShomerUI *u,int screen,int next,const char *next_label,int backup,int userdata,int erase,int userdata_enabled,const char *directory,int percent,int brom){
    u->rendering=1;
    const char *titles[]={"Check images","Identify phone","Back up","Review","Install","Results"};char *window_title=g_strdup_printf("Shomer Flip 2 Workbench — %s",titles[screen]);gtk_window_set_title(GTK_WINDOW(u->window),window_title);g_free(window_title);
    for(int i=0;i<6;i++){GtkStyleContext *c=gtk_widget_get_style_context(u->sidebar[i]);if(i==screen)gtk_style_context_add_class(c,"active");else gtk_style_context_remove_class(c,"active");}
    gtk_widget_set_visible(u->cards,screen==1);gtk_widget_set_visible(u->options,screen==2);gtk_widget_set_visible(u->progress,screen==0||screen==4);
    gtk_widget_set_visible(u->back,screen==2||screen==3);gtk_widget_set_sensitive(u->next,next);gtk_button_set_label(GTK_BUTTON(u->next),next_label);
    gtk_toggle_button_set_active(GTK_TOGGLE_BUTTON(u->backup),backup);gtk_toggle_button_set_active(GTK_TOGGLE_BUTTON(u->userdata),userdata);gtk_toggle_button_set_active(GTK_TOGGLE_BUTTON(u->erase),erase);
    gtk_widget_set_sensitive(u->userdata,userdata_enabled&&backup);gtk_widget_set_sensitive(u->erase,userdata_enabled);
    if(g_strcmp0(gtk_entry_get_text(GTK_ENTRY(u->directory)),directory)!=0)gtk_entry_set_text(GTK_ENTRY(u->directory),directory);
    gtk_progress_bar_set_fraction(GTK_PROGRESS_BAR(u->progress),percent/100.0);
    gtk_widget_set_visible(u->brom,brom);u->rendering=0;
}
const char *shomer_ui_directory(ShomerUI *u){return gtk_entry_get_text(GTK_ENTRY(u->directory));}
char *shomer_ui_folder(ShomerUI *u,const char *current){GtkWidget *d=gtk_file_chooser_dialog_new("Choose backup folder",GTK_WINDOW(u->window),GTK_FILE_CHOOSER_ACTION_SELECT_FOLDER,"Cancel",GTK_RESPONSE_CANCEL,"Choose",GTK_RESPONSE_ACCEPT,NULL);gtk_file_chooser_set_current_folder(GTK_FILE_CHOOSER(d),current);char *path=NULL;if(gtk_dialog_run(GTK_DIALOG(d))==GTK_RESPONSE_ACCEPT)path=gtk_file_chooser_get_filename(GTK_FILE_CHOOSER(d));gtk_widget_destroy(d);return path;}
int shomer_ui_question(ShomerUI *u,int kind,const char *body,int default_yes,char **answer){
    GtkButtonsType buttons=kind==1?GTK_BUTTONS_YES_NO:GTK_BUTTONS_OK_CANCEL;
    GtkWidget *d=gtk_message_dialog_new(GTK_WINDOW(u->window),GTK_DIALOG_MODAL,GTK_MESSAGE_QUESTION,buttons,"%s",body);
    gtk_window_set_title(GTK_WINDOW(d),"Shomer — confirm action");gtk_dialog_set_default_response(GTK_DIALOG(d),kind==1?(default_yes?GTK_RESPONSE_YES:GTK_RESPONSE_NO):GTK_RESPONSE_OK);
    GtkWidget *entry=NULL;if(kind>=2){entry=gtk_entry_new();gtk_entry_set_activates_default(GTK_ENTRY(entry),TRUE);gtk_container_add(GTK_CONTAINER(gtk_dialog_get_content_area(GTK_DIALOG(d))),entry);gtk_widget_show(entry);gtk_widget_grab_focus(entry);}
    int result=gtk_dialog_run(GTK_DIALOG(d));*answer=entry?g_strdup(gtk_entry_get_text(GTK_ENTRY(entry))):NULL;gtk_widget_destroy(d);return result;
}
int shomer_ui_capture(ShomerUI *u,const char *path){GdkWindow *w=gtk_widget_get_window(u->window);int width=gtk_widget_get_allocated_width(u->window),height=gtk_widget_get_allocated_height(u->window);GdkPixbuf *p=gdk_pixbuf_get_from_window(w,0,0,width,height);if(!p)return 0;gboolean ok=gdk_pixbuf_save(p,path,"png",NULL,NULL);g_object_unref(p);return ok;}
