#include <gtk/gtk.h>
typedef struct ShomerUI ShomerUI;
ShomerUI *shomer_ui_new(const char *version);
void shomer_ui_destroy(ShomerUI *u);
void shomer_ui_pump(void);
int shomer_ui_event(ShomerUI *u);
void shomer_ui_text(ShomerUI *u, int field, const char *text);
void shomer_ui_state(ShomerUI *u, int screen, int next, const char *next_label, int backup, int userdata, int erase, int userdata_enabled, const char *directory, int percent, int brom);
const char *shomer_ui_directory(ShomerUI *u);
char *shomer_ui_folder(ShomerUI *u, const char *current);
int shomer_ui_question(ShomerUI *u, int kind, const char *body, int default_yes, char **answer);
int shomer_ui_capture(ShomerUI *u, const char *path);
