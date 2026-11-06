#!/usr/bin/env python3
"""Drives the Windows window of a REAL shomer-flasher .exe like a person, with
real keyboard and mouse input (SendInput through pywinauto). No phone is
connected, so the flasher cannot read the software version and asks for it.

  python test/e2e_gui_windows.py <shomer-flasher .exe> <screenshot dir>

Steps (Sam's report, Nov 4 2026: typed UPCI with no phone, flasher saw ""):
  1. start the window, wait for the ROM check, click "Continue: find the phone"
  2. version box appears; press Enter on an empty box -> it must say what is accepted
  3. type " upci " (small letters, spaces) and click OK with the mouse
  4. the flasher must accept it ("OK - UPCI.") and go on to the next step
     ("Press Enter when the phone is OFF and UNPLUGGED")
  5. press Cancel there -> the window shows "STOPPED: you pressed Cancel", Close
Run 2 (1.4.3, never block): three answers that are not a version -> a WARNING (not a
refusal) -> the person's own choice "upci" -> "Flash anyway as UPCI?" -> click Yes ->
the flow goes on to the power-off step -> Cancel, Close.
Nothing is ever written: there is no phone, and the flow stops before any write.
"""
import glob, os, sys, time, traceback

from pywinauto import Desktop, keyboard
from pywinauto.application import Application

EXE = os.path.abspath(sys.argv[1])
SHOTS = os.path.abspath(sys.argv[2] if len(sys.argv) > 2 else "gui-e2e")
os.makedirs(SHOTS, exist_ok=True)
WORK = os.path.join(os.environ.get("RUNNER_TEMP", os.environ.get("TEMP", ".")), "sf-gui-e2e")
results = []


def check(cond, why):
    results.append((bool(cond), why))
    print(("PASS " if cond else "FAIL ") + why, flush=True)
    return cond


def shot(win, name):
    try:
        win.capture_as_image().save(os.path.join(SHOTS, name + ".png"))
    except Exception as e:  # screenshots are evidence, not the test
        print("  (screenshot %s failed: %s)" % (name, e))


def wait(fn, timeout, what, step=0.5):
    t0 = time.time()
    while time.time() - t0 < timeout:
        try:
            v = fn()
            if v:
                return v
        except Exception:
            pass
        time.sleep(step)
    raise TimeoutError("timed out after %ds waiting for %s" % (timeout, what))


def all_text(w):
    out = []
    for c in [w] + w.descendants():
        try:
            t = c.window_text()
            if t:
                out.append(t)
        except Exception:
            pass
    return "\n".join(out)


def tops(pid):
    return [w for w in Desktop(backend="win32").windows(process=pid) if w.is_visible()]


def text_dialog(pid):
    for w in tops(pid):
        if w.window_text() == "Shomer flasher" and any(c.class_name() == "Edit" for c in w.descendants()):
            return w
    return None


def msgbox(pid, needle):
    for w in tops(pid):
        if w.class_name() == "#32770" and needle in all_text(w):
            return w
    return None


def click(ctrl, name):
    """A real mouse click; if the desktop refuses real input, say so and use BM_CLICK."""
    try:
        ctrl.click_input()
        return "mouse"
    except Exception as e:
        print("  real mouse click on %s failed (%s); using BM_CLICK" % (name, e))
        ctrl.click()
        return "bm_click"


os.makedirs(WORK, exist_ok=True)
app = Application(backend="win32").start('"%s" --ui --workdir "%s" --backup-dir "%s"' % (EXE, os.path.join(WORK, "w"), os.path.join(WORK, "b")),
                                          work_dir=os.path.dirname(EXE), wait_for_idle=False)  # a console program that opens a window
pid = app.process
main = None
try:
    main = wait(lambda: next((w for w in tops(pid) if w.window_text().startswith("Shomer Flip 2 Workbench ")), None), 90, "the main window")
    check(True, "the window opened: %r" % main.window_text())
    shot(main, "1-checks")
    cont = wait(lambda: next((b for b in main.descendants() if b.class_name() == "Button" and b.window_text().startswith("Continue: find the phone") and b.is_enabled()), None),
                900, "the ROM check to finish (Continue enabled)", 2)
    shot(main, "2-checks-done")
    how = click(cont, "Continue")
    print("  Continue clicked by", how)

    # --- the version box, no phone connected ---
    dlg = wait(lambda: text_dialog(pid), 120, "the software-version box")
    body = all_text(dlg)
    check("Type the 4 letters you see (KEKA/UPCI)" in body, "the version box asks for the 4 letters (no phone connected)")
    shot(dlg, "3-version-box")
    edit = next(c for c in dlg.descendants() if c.class_name() == "Edit")
    edit.set_focus()
    keyboard.send_keys("{ENTER}")  # empty answer
    time.sleep(1.5)
    dlg2 = wait(lambda: text_dialog(pid), 30, "the version box again after an empty answer")
    body2 = all_text(dlg2)
    check("Nothing was typed. Accepted answers: KEKA, UPCI" in body2, "an empty answer is refused and the box says what is accepted")
    shot(dlg2, "4-version-box-empty-refused")

    edit = next(c for c in dlg2.descendants() if c.class_name() == "Edit")
    click(edit, "the edit box")
    keyboard.send_keys(" upci ", with_spaces=True, pause=0.08)  # typed like a person, small letters and spaces
    time.sleep(0.5)
    check(edit.window_text() == " upci ", "the edit box shows what was typed: %r" % edit.window_text())
    shot(dlg2, "5-typed-upci")
    okb = next(c for c in dlg2.descendants() if c.class_name() == "Button" and c.window_text() == "OK")
    how = click(okb, "OK")
    print("  OK clicked by", how)

    # --- the flasher must have accepted it and gone on ---
    nxt = wait(lambda: msgbox(pid, "Press Enter when the phone is OFF and UNPLUGGED"), 60, "the next step (power off + unplug)")
    check(True, "after UPCI the flow went on to 'Press Enter when the phone is OFF and UNPLUGGED'")
    shot(nxt, "6-next-step")
    mt = all_text(main)
    check("OK - UPCI." in mt, "the window log shows 'OK - UPCI.'")
    check("is not a supported software version" not in mt, "the typed answer was not refused")
    cancel = next(c for c in nxt.descendants() if c.class_name() == "Button" and c.window_text() == "Cancel")
    click(cancel, "Cancel")

    done = wait(lambda: "STOPPED: you pressed Cancel" in all_text(main), 60, "the Stopped screen")
    check(done, "Cancel stops the flasher and says so ('STOPPED: you pressed Cancel')")
    shot(main, "7-stopped")
    close = next(b for b in main.descendants() if b.class_name() == "Button" and b.window_text() == "Close")
    click(close, "Close")
    wait(lambda: not app.is_process_running(), 60, "the program to exit")
    check(True, "the program closed")
except Exception as e:
    traceback.print_exc()
    check(False, "exception: %s" % e)
    for w in tops(pid) if app.is_process_running() else []:
        print("---- window %r (%s)\n%s" % (w.window_text(), w.class_name(), all_text(w)))
        shot(w, "fail-" + w.class_name().replace("\\", "").replace("/", "")[:20])
finally:
    if app.is_process_running():
        app.kill()
    logs = sorted(glob.glob(os.path.join(os.path.dirname(EXE), "shomer-flasher-*.log")), key=os.path.getmtime)
    if logs:
        log = open(logs[-1], encoding="utf-8", errors="replace").read()
        check("answered {Yes:false Text: upci  Cancel:false}" in log, "the log file shows the answer arrived as ' upci ' (not \"\")")
        check("OK - UPCI." in log, "the log file shows 'OK - UPCI.'")
        writes = [w for w in ("flash recovery", "flash super", "flash vbmeta", "flashing unlock", "oem unlock", "erase ") if w in log]
        check(not writes, "the log shows no write command at all %s" % (writes or ""))
        print("---- log tail\n" + "\n".join(log.splitlines()[-40:]))
    else:
        check(False, "no log file next to the exe")


# ---- 2. never block (Sam, Nov 4 2026): an unknown version -> warning -> the person's own choice -> "Flash anyway as UPCI?" -> Yes
def type_and_ok(dlg, text):
    edit = next(c for c in dlg.descendants() if c.class_name() == "Edit")
    click(edit, "the edit box")
    keyboard.send_keys(text, with_spaces=True, pause=0.08)
    time.sleep(0.4)
    okb = next(c for c in dlg.descendants() if c.class_name() == "Button" and c.window_text() == "OK")
    click(okb, "OK")
    time.sleep(1.5)


print("\n==== 2. unknown version: warning, own choice, Yes", flush=True)
app = Application(backend="win32").start('"%s" --ui --workdir "%s" --backup-dir "%s"' % (EXE, os.path.join(WORK, "w"), os.path.join(WORK, "b")),
                                          work_dir=os.path.dirname(EXE), wait_for_idle=False)
pid = app.process
try:
    main = wait(lambda: next((w for w in tops(pid) if w.window_text().startswith("Shomer Flip 2 Workbench ")), None), 90, "the main window (run 2)")
    cont = wait(lambda: next((b for b in main.descendants() if b.class_name() == "Button" and b.window_text().startswith("Continue: find the phone") and b.is_enabled()), None),
                900, "the ROM check to finish (run 2)", 2)
    main.set_focus()
    time.sleep(1)
    print("  Continue clicked by", click(cont, "Continue"))
    try:
        dlg = wait(lambda: text_dialog(pid), 20, "the version box (try 1)")
    except TimeoutError:  # the click landed before the window had the focus: press the button again
        print("  no version box 20 s after the click; pressing Continue again (BM_CLICK)")
        cont.click()
        dlg = wait(lambda: text_dialog(pid), 60, "the version box (try 1, second press)")
    for i, typed in enumerate(["xx", "KEE7", "abc"]):  # three answers that are not a supported version
        if i > 0:
            dlg = wait(lambda: text_dialog(pid), 60, "the version box (try %d)" % (i + 1))
        type_and_ok(dlg, typed)
    dlg = wait(lambda: text_dialog(pid), 60, "the 'flash it as' box after the warning")
    body = all_text(dlg)
    check("WARNING - THE PHONE COULD NOT BE CONFIRMED" in body and "abc" in body, "after 3 unknown answers: a WARNING naming what was typed, not a refusal")
    check("type the version to flash it as" in body, "the box asks for the person's own choice (KEKA or UPCI)")
    shot(dlg, "8-variant-warning-choice")
    type_and_ok(dlg, "upci")
    box = wait(lambda: msgbox(pid, "Flash anyway as UPCI?"), 60, "the 'Flash anyway as UPCI?' question")
    check("WARNING - THE PHONE COULD NOT BE CONFIRMED" in all_text(box), "the yes/no box shows the warning too")
    shot(box, "9-flash-anyway")
    yes = next(c for c in box.descendants() if c.class_name() == "Button" and c.window_text().replace("&", "") == "Yes")
    how = click(yes, "Yes")
    print("  Yes clicked by", how)
    nxt = wait(lambda: msgbox(pid, "Press Enter when the phone is OFF and UNPLUGGED"), 60, "the next step after Yes")
    check(True, "warning + Yes: the flow went on to 'Press Enter when the phone is OFF and UNPLUGGED'")
    shot(nxt, "10-after-yes")
    cancel = next(c for c in nxt.descendants() if c.class_name() == "Button" and c.window_text() == "Cancel")
    click(cancel, "Cancel")
    wait(lambda: "STOPPED: you pressed Cancel" in all_text(main), 60, "the Stopped screen (run 2)")
    close = next(b for b in main.descendants() if b.class_name() == "Button" and b.window_text() == "Close")
    main.set_focus()
    click(close, "Close")
    try:
        wait(lambda: not app.is_process_running(), 15, "the program to exit (run 2)")
    except TimeoutError:  # same as Continue above: the mouse click did not reach the button; press it
        print("  still running 15 s after the Close click; pressing Close again (BM_CLICK)")
        close.click()
        wait(lambda: not app.is_process_running(), 60, "the program to exit (run 2, second press)")
except Exception as e:
    traceback.print_exc()
    check(False, "run 2 exception: %s" % e)
    for w in tops(pid) if app.is_process_running() else []:
        print("---- window %r (%s)\n%s" % (w.window_text(), w.class_name(), all_text(w)))
        shot(w, "fail2-" + w.class_name().replace("\\", "").replace("/", "")[:20])
finally:
    if app.is_process_running():
        app.kill()
    logs = sorted(glob.glob(os.path.join(os.path.dirname(EXE), "shomer-flasher-*.log")), key=os.path.getmtime)
    log = open(logs[-1], encoding="utf-8", errors="replace").read() if logs else ""
    check("variant warning overridden by the person as UPCI" in log, "run 2 log: the person chose to go on as UPCI after the warning")
    writes = [w for w in ("flash recovery", "flash super", "flash vbmeta", "flashing unlock", "oem unlock", "erase ") if w in log]
    check(log and not writes, "run 2 log shows no write command at all %s" % (writes or ""))

bad = [w for ok, w in results if not ok]
print("\n%d/%d checks passed" % (len(results) - len(bad), len(results)))
sys.exit(1 if bad else 0)
