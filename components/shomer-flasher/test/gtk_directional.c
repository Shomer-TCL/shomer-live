/* Exercise the production GTK key handler and real focus chain under Xvfb. */
#include "../internal/gui/gtk_linux.c"
#include <assert.h>
static void key(ShomerUI *u, guint value) {
    GdkEventKey event = {0}; event.keyval = value;
    directional_key(u->window, &event, u);
    shomer_ui_pump();
}
int main(int argc, char **argv) {
    gtk_init(&argc, &argv);
    ShomerUI *u = shomer_ui_new("keypad regression");
    shomer_ui_state(u, 2, 1, "Continue", 1, 0, 0, 1, "/media/backup", 0, 1);
    shomer_ui_pump();
    gtk_widget_grab_focus(u->backup);
    gboolean before = gtk_toggle_button_get_active(GTK_TOGGLE_BUTTON(u->backup));
    key(u, GDK_KEY_Return);
    assert(gtk_toggle_button_get_active(GTK_TOGGLE_BUTTON(u->backup)) != before);
    assert(gtk_window_get_focus(GTK_WINDOW(u->window)) == u->backup);
    key(u, GDK_KEY_Return); /* Restore backup on, so dependent options remain enabled. */
    key(u, GDK_KEY_Down);
    assert(gtk_window_get_focus(GTK_WINDOW(u->window)) == u->directory);
    key(u, GDK_KEY_Right); /* Text cursor keeps its native behavior. */
    assert(gtk_window_get_focus(GTK_WINDOW(u->window)) == u->directory);
    key(u, GDK_KEY_Down); /* Leave the editable path through its vertical navigation. */
    gboolean saw_userdata=FALSE, saw_erase=FALSE, saw_next=FALSE, saw_brom=FALSE;
    for(int i=0;i<24;i++) {
        key(u,GDK_KEY_Right);
        GtkWidget *focus=gtk_window_get_focus(GTK_WINDOW(u->window));
        saw_userdata |= focus==u->userdata; saw_erase |= focus==u->erase;
        saw_next |= focus==u->next; saw_brom |= focus==u->brom;
    }
    assert(saw_userdata && saw_erase && saw_next && saw_brom);
    gtk_widget_grab_focus(u->directory); key(u,GDK_KEY_Up);
    assert(gtk_window_get_focus(GTK_WINDOW(u->window)) == u->backup);
    gtk_widget_grab_focus(u->body); key(u,GDK_KEY_Right);
    assert(gtk_window_get_focus(GTK_WINDOW(u->window)) != u->body);
    shomer_ui_destroy(u);
    puts("PASS directional focus, editable path, checkboxes and text escape");
    return 0;
}
