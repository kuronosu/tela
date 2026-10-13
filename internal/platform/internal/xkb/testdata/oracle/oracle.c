/*
 * oracle.c generates the expected results of the xkb package's tests from
 * libxkbcommon, the reference implementation. It is a development tool, not
 * part of Tela: it lives under testdata so the Go tool never builds it, and
 * nothing in Tela links libxkbcommon (no cgo). See ../README for how the
 * fixtures were produced.
 *
 * Build:  cc -O2 -o oracle oracle.c $(pkg-config --cflags --libs xkbcommon)
 *
 * Modes:
 *
 *   oracle keymap FILE LABEL=MODS:LOCKED:GROUP...
 *     Loads FILE (format v1, like a Wayland client) and prints the masks of
 *     the named modifiers, the keys that do not repeat, and for every state
 *     given on the command line the keysym (xkb_state_key_get_one_sym, with
 *     the Caps Lock transformation) and the UTF-8 text
 *     (xkb_state_key_get_utf8) of every key that has either. MODS and
 *     LOCKED are modifier names joined with '+' (depressed and locked
 *     masks); GROUP is the effective layout index.
 *
 *   oracle syms
 *     Reads keysyms (hex, one per line) on stdin and prints, for each,
 *     xkb_keysym_to_utf32, xkb_keysym_to_upper and xkb_keysym_to_lower.
 *
 *   oracle compose FILE LOCALE
 *     Loads the Compose file FILE and reads lines of keysym names on stdin;
 *     each line is fed to a fresh compose state, and the status after each
 *     keysym and the final UTF-8 result are printed.
 */
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <xkbcommon/xkbcommon.h>
#include <xkbcommon/xkbcommon-compose.h>

static const char *mod_names[] = {
    "Shift", "Lock", "Control", "Mod1", "Mod2", "Mod3", "Mod4", "Mod5",
    "NumLock", "Alt", "LevelThree", "LevelFive", "Super", "Meta", "Hyper",
    "ScrollLock", NULL,
};

static char *read_file(const char *path) {
    FILE *f = fopen(path, "rb");
    if (!f) { perror(path); exit(1); }
    fseek(f, 0, SEEK_END);
    long n = ftell(f);
    fseek(f, 0, SEEK_SET);
    char *buf = malloc(n + 1);
    if (fread(buf, 1, n, f) != (size_t)n) { perror(path); exit(1); }
    buf[n] = 0;
    fclose(f);
    return buf;
}

static xkb_mod_mask_t parse_mods(struct xkb_keymap *km, const char *s) {
    xkb_mod_mask_t m = 0;
    char buf[256];
    snprintf(buf, sizeof buf, "%s", s);
    for (char *tok = strtok(buf, "+"); tok; tok = strtok(NULL, "+")) {
        xkb_mod_mask_t v = xkb_keymap_mod_get_mask(km, tok);
        if (v == 0) {
            fprintf(stderr, "unknown or unmapped modifier %s\n", tok);
            exit(1);
        }
        m |= v;
    }
    return m;
}

static void print_utf8(const char *s) {
    if (!*s) { printf("-"); return; }
    for (const unsigned char *p = (const unsigned char *)s; *p; p++)
        printf("%02x", *p);
}

static int keymap_mode(int argc, char **argv) {
    struct xkb_context *ctx = xkb_context_new(XKB_CONTEXT_NO_DEFAULT_INCLUDES | XKB_CONTEXT_NO_ENVIRONMENT_NAMES);
    char *text = read_file(argv[0]);
    struct xkb_keymap *km = xkb_keymap_new_from_string(ctx, text, XKB_KEYMAP_FORMAT_TEXT_V1, 0);
    if (!km) { fprintf(stderr, "cannot compile %s\n", argv[0]); return 1; }
    for (int i = 0; mod_names[i]; i++) {
        if (xkb_keymap_mod_get_index(km, mod_names[i]) == XKB_MOD_INVALID)
            continue;
        printf("mask %s %#x\n", mod_names[i], xkb_keymap_mod_get_mask(km, mod_names[i]));
    }
    printf("layouts %u\n", xkb_keymap_num_layouts(km));
    xkb_keycode_t min = xkb_keymap_min_keycode(km), max = xkb_keymap_max_keycode(km);
    printf("norepeat");
    for (xkb_keycode_t kc = min; kc <= max; kc++)
        if (xkb_keymap_num_layouts_for_key(km, kc) > 0 && !xkb_keymap_key_repeats(km, kc))
            printf(" %u", kc);
    printf("\n");
    struct xkb_state *st = xkb_state_new(km);
    for (int a = 1; a < argc; a++) {
        char label[64], mods[128], locked[128];
        unsigned group;
        const char *eq = strchr(argv[a], '=');
        const char *c1 = eq ? strchr(eq + 1, ':') : NULL;
        const char *c2 = c1 ? strchr(c1 + 1, ':') : NULL;
        if (!eq || !c1 || !c2) { fprintf(stderr, "bad state %s\n", argv[a]); return 1; }
        snprintf(label, sizeof label, "%.*s", (int)(eq - argv[a]), argv[a]);
        snprintf(mods, sizeof mods, "%.*s", (int)(c1 - eq - 1), eq + 1);
        snprintf(locked, sizeof locked, "%.*s", (int)(c2 - c1 - 1), c1 + 1);
        group = (unsigned)atoi(c2 + 1);
        xkb_mod_mask_t dep = parse_mods(km, mods), lock = parse_mods(km, locked);
        xkb_state_update_mask(st, dep, 0, lock, 0, 0, group);
        printf("state %s %#x %#x %u\n", label, dep, lock, group);
        for (xkb_keycode_t kc = min; kc <= max; kc++) {
            xkb_keysym_t sym = xkb_state_key_get_one_sym(st, kc);
            char buf[64];
            xkb_state_key_get_utf8(st, kc, buf, sizeof buf);
            if (sym == XKB_KEY_NoSymbol && !buf[0])
                continue;
            printf("%u %#x ", kc, sym);
            print_utf8(buf);
            printf("\n");
        }
    }
    return 0;
}

static int syms_mode(void) {
    char line[64];
    while (fgets(line, sizeof line, stdin)) {
        xkb_keysym_t ks = (xkb_keysym_t)strtoul(line, NULL, 16);
        printf("%#x %#x %#x %#x\n", ks, xkb_keysym_to_utf32(ks), xkb_keysym_to_upper(ks), xkb_keysym_to_lower(ks));
    }
    return 0;
}

static int compose_mode(char **argv) {
    struct xkb_context *ctx = xkb_context_new(XKB_CONTEXT_NO_DEFAULT_INCLUDES | XKB_CONTEXT_NO_ENVIRONMENT_NAMES);
    FILE *f = fopen(argv[0], "r");
    if (!f) { perror(argv[0]); return 1; }
    struct xkb_compose_table *t = xkb_compose_table_new_from_file(ctx, f, argv[1], XKB_COMPOSE_FORMAT_TEXT_V1, 0);
    if (!t) { fprintf(stderr, "cannot compile %s\n", argv[0]); return 1; }
    static const char *status[] = {"nothing", "composing", "composed", "cancelled"};
    char line[1024];
    while (fgets(line, sizeof line, stdin)) {
        line[strcspn(line, "\n")] = 0;
        if (!line[0])
            continue;
        struct xkb_compose_state *st = xkb_compose_state_new(t, 0);
        printf("%s =>", line);
        for (char *tok = strtok(line, " "); tok; tok = strtok(NULL, " ")) {
            xkb_keysym_t ks = xkb_keysym_from_name(tok, 0);
            if (xkb_compose_state_feed(st, ks) == XKB_COMPOSE_FEED_IGNORED) {
                printf(" ignored");
                continue;
            }
            printf(" %s", status[xkb_compose_state_get_status(st)]);
        }
        char buf[256];
        xkb_compose_state_get_utf8(st, buf, sizeof buf);
        printf(" ");
        print_utf8(buf);
        printf("\n");
        xkb_compose_state_unref(st);
    }
    return 0;
}

int main(int argc, char **argv) {
    if (argc >= 3 && !strcmp(argv[1], "keymap"))
        return keymap_mode(argc - 2, argv + 2);
    if (argc == 2 && !strcmp(argv[1], "syms"))
        return syms_mode();
    if (argc == 4 && !strcmp(argv[1], "compose"))
        return compose_mode(argv + 2);
    fprintf(stderr, "usage: oracle keymap FILE STATE... | oracle syms | oracle compose FILE LOCALE\n");
    return 2;
}
