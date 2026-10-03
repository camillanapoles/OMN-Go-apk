package net.basov.omngo;

// IntentBridge opens an "intent:" link of a note. It also holds the two
// ways back: the result of a capture, for example a barcode scan, and the
// output of a Termux command.
//
// WebViewSetup calls handleIntentUri. MainActivity calls register,
// unregister, saveState, restoreState and handleCaptureResult from the
// methods of its life cycle.
//
// A member here is not private. See the banner of ShareIn for the reason.
final class IntentBridge {
    final MainActivity a;

    IntentBridge(MainActivity activity) {
        a = activity;
    }

    // ----------------------------------------------------------------------
    // Android intent-URI links (incl. Termux RUN_COMMAND integration)
    // ----------------------------------------------------------------------
    //
    // Reached from WebViewSetup when the URL of a note link
    // starts with "intent:". It reproduces the behavior of the pre-OMN-Go
    // app, mvbasov/OMN, and the Termux argument-packing convention of that
    // app with it. An intent: link in a note authored for that app thus
    // keeps working here unchanged.
    //
    // Two independent config toggles gate this. Both are off by default,
    // and readConfigFlag reads them live from config.json at tap time. A
    // Settings change thus applies on the next tap, with no app restart:
    //   - enable_intent_uri   : master switch. Off => no intent link launches.
    //   - enable_termux_intent : additionally allows the Termux RUN_COMMAND
    //                            path (a note starting a shell command via
    //                            com.termux/.app.RunCommandService).
    //
    // The Termux path is deliberately hardened beyond old OMN. FOUR
    // independent consents come before a note can run a shell command.
    // The master toggle is on, the Termux toggle is on, the RUN_COMMAND
    // permission is granted, and a per-tap confirmation dialog is
    // answered.
    //
    // The reason is that an OMN-Go note is not necessarily self-authored.
    // Git sync can put third-party content in front of this WebView. So
    // can another device that edits over the network while LAN sharing is
    // on. A single tap must never silently run a shell command.

    private static final int REQ_TERMUX_PERMISSION = 1003;
    private static final String TERMUX_PACKAGE = "com.termux";
    private static final String TERMUX_PERMISSION = "com.termux.permission.RUN_COMMAND";
    private static final String TERMUX_RUN_COMMAND_PATH = "com.termux.RUN_COMMAND_PATH";
    private static final String TERMUX_RUN_COMMAND_ARGS = "com.termux.RUN_COMMAND_ARGUMENTS";
    private static final String TERMUX_RUN_COMMAND_LABEL = "com.termux.RUN_COMMAND_LABEL";

    // Activity-result capture, for example a barcode scan pasted into
    // Quick Notes. OMNGO_CAPTURE_EXTRA is the private marker of OMN-Go on
    // an intent: URI, and its value names the result extra to read back.
    // REQ_CAPTURE_RESULT tags the startActivityForResult call, thus
    // onActivityResult can recognize it. pendingCaptureExtra holds that
    // result-extra name across the launch. It is round-tripped through
    // onSaveInstanceState as STATE_PENDING_CAPTURE_EXTRA, thus it survives
    // a kill of the process while the launched activity is in the
    // foreground.
    static final int REQ_CAPTURE_RESULT = 1004;
    private static final String OMNGO_CAPTURE_EXTRA = "omngo_capture_extra";
    private static final String STATE_PENDING_CAPTURE_EXTRA = "omngo_pending_capture_extra";
    private String pendingCaptureExtra;

    // Termux command-output capture (a note's shell command whose stdout/stderr
    // is pasted back into a review dialog). Opt-in per note via
    // OMNGO_CAPTURE_OUTPUT, whose value names the stream(s): "stdout" (default),
    // "stderr", or "both". Termux returns the result asynchronously through a
    // PendingIntent we supply (RUN_COMMAND_PENDING_INTENT), NOT via an activity
    // result - hence a broadcast receiver rather than onActivityResult. The
    // TERMUX_RESULT_* keys are the literal values from Termux's TermuxConstants
    // (verified against termux-app master): the result Bundle lives under the
    // "result" extra and carries stdout/stderr/exitCode/errmsg. OMNGO_STREAM /
    // OMNGO_LABEL ride on our own PendingIntent's base intent so the receiver
    // gets them back alongside Termux's bundle - no cross-call state to hold.
    private static final String OMNGO_CAPTURE_OUTPUT = "omngo_capture_output";
    private static final String TERMUX_RUN_COMMAND_BACKGROUND = "com.termux.RUN_COMMAND_BACKGROUND";
    private static final String TERMUX_RUN_COMMAND_PENDING_INTENT = "com.termux.RUN_COMMAND_PENDING_INTENT";
    private static final String TERMUX_RESULT_BUNDLE = "result";
    private static final String TERMUX_RESULT_STDOUT = "stdout";
    private static final String TERMUX_RESULT_STDERR = "stderr";
    private static final String TERMUX_RESULT_EXIT_CODE = "exitCode";
    private static final String ACTION_TERMUX_RESULT = "net.basov.omngo.TERMUX_RESULT";
    private static final String OMNGO_STREAM = "omngo_stream";
    private static final String OMNGO_LABEL = "omngo_label";
    private android.content.BroadcastReceiver termuxResultReceiver;
    // A distinct request code for each capture launch. The result
    // PendingIntents of two concurrent commands then do not collide under
    // FLAG_UPDATE_CURRENT.
    private int termuxResultRequestCounter = 5000;

    // register starts to listen for the result of a Termux command. See
    // the capture path of launchTermuxIntent. The receiver is for this
    // package only, NOT_EXPORTED, the same as the receiver of Shortcuts.
    // It is dynamic, and thus activity-scoped. When the OS kills this
    // process while a long command still runs, the result cannot be
    // delivered. The command still runs, and only the paste-back dialog is
    // lost. Accepted for v1, because a typical captured command finishes
    // in well under a second.
    void register() {
        termuxResultReceiver = new android.content.BroadcastReceiver() {
            @Override
            public void onReceive(android.content.Context context, android.content.Intent intent) {
                handleTermuxResult(intent);
            }
        };
        android.content.IntentFilter termuxResultFilter = new android.content.IntentFilter(ACTION_TERMUX_RESULT);
        if (android.os.Build.VERSION.SDK_INT >= 33) {
            a.registerReceiver(termuxResultReceiver, termuxResultFilter, android.content.Context.RECEIVER_NOT_EXPORTED);
        } else {
            a.registerReceiver(termuxResultReceiver, termuxResultFilter);
        }
    }

    void unregister() {
        if (termuxResultReceiver != null) {
            try {
                a.unregisterReceiver(termuxResultReceiver);
            } catch (Exception e) {
                // Already unregistered, or never registered at all. Either
                // way there is nothing left to clean up.
            }
        }
    }

    // restoreState gets the "which result extra were we waiting for" marker
    // back. onCreate calls it as early as possible, before onActivityResult
    // can fire. The process can be killed while a capture activity is in
    // the foreground, for example the barcode scanner. See
    // launchCaptureIntent and handleCaptureResult.
    void restoreState(android.os.Bundle savedInstanceState) {
        if (savedInstanceState != null) {
            pendingCaptureExtra = savedInstanceState.getString(STATE_PENDING_CAPTURE_EXTRA);
        }
    }

    // saveState keeps which result extra the code waits for. A capture,
    // for example a barcode scan, still pastes into Quick Notes. That
    // holds when the OS killed this process while the scanner was in the
    // foreground.
    void saveState(android.os.Bundle outState) {
        if (pendingCaptureExtra != null) {
            outState.putString(STATE_PENDING_CAPTURE_EXTRA, pendingCaptureExtra);
        }
    }

    void handleIntentUri(final String url) {
        if (!a.readConfigFlag("enable_intent_uri")) {
            a.showToast("Intent links are turned off. Enable them in Settings.");
            return;
        }

        final android.content.Intent intentApp;
        try {
            // parseUri() handles the bare "intent:#Intent;...;end" form and
            // the "intent://...#Intent;...;end" form. It also
            // percent-decodes a string extra value. A "%20" inside a
            // packed Termux argument thus becomes a space here, and the
            // packing convention below depends on that.
            intentApp = android.content.Intent.parseUri(url, android.content.Intent.URI_INTENT_SCHEME);
        } catch (Exception e) {
            e.printStackTrace();
            a.showToast("This intent link is malformed.");
            return;
        }

        // The presence of the RUN_COMMAND_PATH extra marks this as a Termux
        // "run a shell command" intent, and not an ordinary one. It is
        // checked first, because the output of a Termux command comes back
        // by a different mechanism than an activity result. That mechanism
        // is a future feature, and it is not the omngo_capture_extra path
        // below.
        if (intentApp.hasExtra(TERMUX_RUN_COMMAND_PATH)) {
            launchTermuxIntent(intentApp);
            return;
        }

        // OMN-Go's own opt-in marker for "launch this for a result and paste
        // it into Quick Notes". Its value names the result extra to read back
        // (e.g. "SCAN_RESULT" for a barcode scan). Only when a note explicitly
        // carries it do we wait for a result; every other intent stays
        // fire-and-forget. See launchCaptureIntent / handleCaptureResult.
        String captureExtra = intentApp.getStringExtra(OMNGO_CAPTURE_EXTRA);
        if (captureExtra != null && !captureExtra.isEmpty()) {
            launchCaptureIntent(intentApp, captureExtra);
        } else {
            launchGenericIntent(intentApp);
        }
    }

    /**
     * Lets a "file://" URI leave this process.
     *
     * An application that targets API 24 or later gets a VmPolicy with
     * penaltyDeathOnFileUriExposure, so startActivity() on an intent whose
     * data is a file:// URI throws FileUriExposedException before the other
     * application is ever asked. Clearing the policy is the documented way
     * out for an application that hands a real path to another one on
     * purpose, which is what both callers of this do.
     *
     * WHAT IT COSTS. StrictMode is a development aid, not a permission
     * boundary: nothing here widens what OMN-Go may read or write. It only
     * stops the platform from killing this process for passing a path. The
     * receiving application still needs its own storage permission to open
     * the file, and a file it may not read stays a file it may not read.
     *
     * The policy is process-wide and there is no per-call form of it, so
     * this is called at the point of use rather than at startup: on a run
     * that never opens an external editor and never follows a file: intent
     * link, the default policy stays in force.
     */
    static void allowFileUriHandoff() {
        android.os.StrictMode.setVmPolicy(new android.os.StrictMode.VmPolicy.Builder().build());
    }

    // An ordinary intent, and not a Termux one. Hand it to the OS as an
    // activity, for example an android.settings.* screen or a third-party
    // app deep link. resolveActivity() is deliberately NOT used as a
    // pre-check here. Under API 30+ package visibility it can return null
    // even for an action that the system itself would handle, and some
    // android.settings.* screens are among those. A try and catch around
    // startActivity is the reliable form. This honors the standard
    // S.browser_fallback_url extra, which loads in the WebView, when no
    // installed app can handle the intent.
    void launchGenericIntent(final android.content.Intent intentApp) {
        String fallbackUrl = intentApp.getStringExtra("browser_fallback_url");
        // Do not leave the fallback URL in the extras of the launched
        // intent, where the target activity can misread it.
        intentApp.removeExtra("browser_fallback_url");
        // A note may point at a real path, for example a photo in DCIM or
        // a PDF in Documents. That is a file:// URI by the time parseUri is
        // finished with it. See allowFileUriHandoff for why this is
        // necessary and what it does not change.
        android.net.Uri data = intentApp.getData();
        if (data != null && "file".equals(data.getScheme())) {
            allowFileUriHandoff();
        }
        try {
            a.startActivity(intentApp);
        } catch (android.content.ActivityNotFoundException e) {
            if (fallbackUrl != null
                    && (fallbackUrl.startsWith("http://") || fallbackUrl.startsWith("https://"))) {
                if (a.webView != null) {
                    a.webView.loadUrl(fallbackUrl);
                }
            } else {
                a.showToast("No app can handle this link.");
            }
        } catch (Exception e) {
            e.printStackTrace();
            a.showToast("Couldn't open this link.");
        }
    }

    // Capture path: launch the target app FOR A RESULT, remembering which
    // result extra to read back when it finishes. Only reached when a note
    // opted in with omngo_capture_extra (see handleIntentUri). No confirmation
    // dialog: unlike Termux, this launches an ordinary app UI (a scanner) and
    // only pastes text - it runs nothing on the device.
    void launchCaptureIntent(final android.content.Intent intentApp, final String captureExtra) {
        // FLAG_ACTIVITY_NEW_TASK (and its NEW_DOCUMENT / MULTIPLE_TASK
        // relatives) start the target in a separate task, which severs the
        // result chain so onActivityResult would never fire. Clear them so the
        // result actually comes back to us, whatever flags the parsed URI set.
        intentApp.setFlags(intentApp.getFlags()
                & ~android.content.Intent.FLAG_ACTIVITY_NEW_TASK
                & ~android.content.Intent.FLAG_ACTIVITY_NEW_DOCUMENT
                & ~android.content.Intent.FLAG_ACTIVITY_MULTIPLE_TASK);
        // Do not leak the private marker of OMN-Go to the target app.
        intentApp.removeExtra(OMNGO_CAPTURE_EXTRA);
        pendingCaptureExtra = captureExtra;
        try {
            a.startActivityForResult(intentApp, REQ_CAPTURE_RESULT);
        } catch (android.content.ActivityNotFoundException e) {
            pendingCaptureExtra = null;
            a.showToast("No app can handle this link.");
        } catch (Exception e) {
            pendingCaptureExtra = null;
            e.printStackTrace();
            a.showToast("Couldn't open this link.");
        }
    }

    // Handles the return from a capture launch (REQ_CAPTURE_RESULT). Called
    // from onActivityResult. Every "no result" path is handled with care,
    // and those are a cancel, no data, and an absent requested extra.
    // Android delivers this callback even when the launched activity set no
    // result at all, with resultCode == RESULT_CANCELED and data == null.
    // There is thus nothing to crash on. On success the text goes to
    // insertCapturedText, which shows it in a pre-filled review dialog and
    // does not save it silently.
    void handleCaptureResult(int resultCode, android.content.Intent data) {
        final String extraName = pendingCaptureExtra;
        pendingCaptureExtra = null; // consume it either way
        if (extraName == null) {
            // A capture callback with no remembered extra name. The process
            // was killed, for example, and the onSaveInstanceState state was
            // not restored. Nothing actionable.
            return;
        }
        if (resultCode != android.app.Activity.RESULT_OK || data == null) {
            // User backed out of the scanner, or the activity returned
            // nothing. Deliberately silent, because a canceled scan is not
            // an error.
            return;
        }
        final String value = extractResultText(data, extraName);
        if (value == null || value.isEmpty()) {
            // The app returned OK but not the extra this note asked for.
            a.showToast("No \"" + extraName + "\" result was returned.");
            return;
        }
        // No label for a scan. The decoded text alone goes to the user for
        // review.
        insertCapturedText(value, null);
    }

    // Reads the named result extra out of a returned Intent. A barcode scan
    // returns a single String (SCAN_RESULT). The reader also accepts a
    // String-ArrayList extra, and it joins the entries of that list. The
    // same path thus works for any result-returning app, with no special
    // case and at no extra cost.
    String extractResultText(android.content.Intent data, String extraName) {
        String s = data.getStringExtra(extraName);
        if (s != null) {
            return s;
        }
        java.util.ArrayList<String> list = data.getStringArrayListExtra(extraName);
        if (list != null && !list.isEmpty()) {
            StringBuilder sb = new StringBuilder();
            for (int i = 0; i < list.size(); i++) {
                if (i > 0) sb.append('\n');
                sb.append(list.get(i));
            }
            return sb.toString();
        }
        return null;
    }

    // Receives a Termux command's result (see launchTermuxIntent's capture
    // path). intent carries our own OMNGO_STREAM / OMNGO_LABEL (from the
    // PendingIntent's base intent) plus Termux's result Bundle. Extracts the
    // requested stream(s) and hands the text to the same review dialog the
    // barcode path uses.
    void handleTermuxResult(android.content.Intent intent) {
        if (intent == null) {
            return;
        }
        String stream = intent.getStringExtra(OMNGO_STREAM);
        String label = intent.getStringExtra(OMNGO_LABEL);
        android.os.Bundle result = intent.getBundleExtra(TERMUX_RESULT_BUNDLE);
        if (result == null) {
            a.showToast("Termux returned no result.");
            return;
        }
        String stdout = result.getString(TERMUX_RESULT_STDOUT);
        String stderr = result.getString(TERMUX_RESULT_STDERR);
        int exitCode = result.getInt(TERMUX_RESULT_EXIT_CODE, 0);
        String text = OmnText.buildCaptureText(stream, stdout, stderr, exitCode);
        if (text == null || text.isEmpty()) {
            a.showToast("Command produced no output.");
            return;
        }
        insertCapturedText(text, (label != null && !label.isEmpty()) ? label : null);
    }

    // Shared "paste a captured result" entry point for both the barcode
    // path (onActivityResult) and the Termux-output path (broadcast). It is
    // never silent. It pre-fills the in-app Quick Note panel
    // (window.omnGoInsertCapture) for the user to review and save. When
    // that panel is not available on the current page, it falls back to a
    // native dialog with the text pre-filled and editable. A page mid-edit
    // on editor.html is such a page, because it loads no omn-go-api.js.
    void insertCapturedText(final String text, final String label) {
        a.runOnUiThread(new Runnable() {
            @Override
            public void run() {
                if (a.webView == null) {
                    showNativeCaptureDialog(text, label);
                    return;
                }
                // URI-encode the values, and decodeURIComponent them in JS.
                // The share handling of onNewIntent uses the same safe
                // transport. Arbitrary text, such as a quote or a newline,
                // then cannot break the JS string.
                String encText = android.net.Uri.encode(text != null ? text : "");
                String encLabel = android.net.Uri.encode(label != null ? label : "");
                String js = "(function(){ try{ return (typeof window.omnGoInsertCapture==='function' && "
                    + "window.omnGoInsertCapture(decodeURIComponent('" + encText + "'), "
                    + "decodeURIComponent('" + encLabel + "'))===true); }catch(e){ return false; } })();";
                a.webView.evaluateJavascript(js, new android.webkit.ValueCallback<String>() {
                    @Override
                    public void onReceiveValue(String value) {
                        // evaluateJavascript returns the JS value JSON-encoded,
                        // so a boolean true arrives as the literal "true".
                        if (!"true".equals(value)) {
                            showNativeCaptureDialog(text, label);
                        }
                    }
                });
            }
        });
    }

    // Native fallback review dialog: the captured text pre-filled in an
    // editable field, saved to Quick Notes only if the user confirms.
    void showNativeCaptureDialog(final String text, final String label) {
        a.runOnUiThread(new Runnable() {
            @Override
            public void run() {
                final android.widget.EditText input = new android.widget.EditText(a);
                String initial = (label != null && !label.isEmpty())
                    ? (label + "\n\n" + (text != null ? text : ""))
                    : (text != null ? text : "");
                input.setText(initial);
                new android.app.AlertDialog.Builder(a)
                    .setTitle("Add to Quick Notes")
                    .setView(input)
                    .setPositiveButton("Save", (d, w) -> {
                        final String note = input.getText().toString();
                        new Thread(new Runnable() {
                            @Override
                            public void run() {
                                try {
                                    a.shareIn.postQuickNoteWithRetry("\n" + note + "\n");
                                    a.showToast("Added to Quick Notes");
                                    a.runOnUiThread(new Runnable() {
                                        @Override
                                        public void run() {
                                            if (a.webView != null) {
                                                String cur = a.webView.getUrl();
                                                if (cur != null && cur.contains("QuickNotes")) {
                                                    a.webView.reload();
                                                }
                                            }
                                        }
                                    });
                                } catch (java.io.IOException e) {
                                    e.printStackTrace();
                                    a.showToast("Couldn't save to Quick Notes.");
                                }
                            }
                        }).start();
                    })
                    .setNegativeButton("Cancel", null)
                    .show();
            }
        });
    }

    // Termux RUN_COMMAND path. Enforces the second toggle + Termux installed +
    // permission granted, applies old OMN's argument-packing convention, then
    // asks for explicit confirmation before starting the service.
    void launchTermuxIntent(final android.content.Intent intentApp) {
        if (!a.readConfigFlag("enable_termux_intent")) {
            a.showToast("Termux commands are turned off. Enable them in Settings.");
            return;
        }
        if (!isPackageInstalled(TERMUX_PACKAGE)) {
            a.showToast("Termux is not installed.");
            return;
        }
        if (a.checkSelfPermission(TERMUX_PERMISSION)
                != android.content.pm.PackageManager.PERMISSION_GRANTED) {
            // Ask now. The user grants it and taps the link again. It stays
            // a re-tap, and it is not an auto-retry through
            // onRequestPermissionsResult. There is thus no cross-callback
            // state to hold for this rare path.
            a.requestPermissions(new String[]{ TERMUX_PERMISSION }, REQ_TERMUX_PERMISSION);
            a.showToast("Grant Termux the RUN_COMMAND permission, then tap the link again.");
            return;
        }

        // Old OMN packing convention. An intent URI cannot carry a String[]
        // extra, thus the arguments are packed into RUN_COMMAND_PATH as
        // "path?arg1&arg2&...". A space inside an argument is %20 there,
        // and parseUri above already decoded it. Unpack into the real
        // RUN_COMMAND_PATH and RUN_COMMAND_ARGUMENTS[] that Termux expects.
        //
        // The boundary between the path and the arguments is split on the
        // FIRST '?' only, with limit 2. That is a safe superset of the
        // unlimited split of old OMN. Every note that worked there had no
        // '?' inside an argument, or it was already broken. The result is
        // thus the same for an existing note, and it is correct as well
        // when an argument itself contains '?'. Arguments are still
        // separated on every '&', as old OMN does. An argument therefore
        // cannot contain a literal '&', which is a documented limitation of
        // the packing.
        String cmdPath = intentApp.getStringExtra(TERMUX_RUN_COMMAND_PATH);
        if (cmdPath != null && cmdPath.contains("?")) {
            String[] cmdParts = cmdPath.split("\\?", 2);
            intentApp.putExtra(TERMUX_RUN_COMMAND_PATH, cmdParts[0].trim());
            if (cmdParts.length > 1 && !cmdParts[1].isEmpty()) {
                intentApp.putExtra(TERMUX_RUN_COMMAND_ARGS, cmdParts[1].split("&"));
            }
        }

        // Opt-in output capture. When the note carries omngo_capture_output,
        // wire up a result PendingIntent. The stdout and stderr of the
        // command then come back to handleTermuxResult, and they are pasted
        // into the review dialog. Left alone, fire-and-forget as before,
        // when the marker is absent.
        if (intentApp.hasExtra(OMNGO_CAPTURE_OUTPUT)) {
            String stream = intentApp.getStringExtra(OMNGO_CAPTURE_OUTPUT);
            if (stream == null || stream.isEmpty()) {
                stream = "stdout"; // default stream when the value is bare
            }
            // Do not leak the private marker of OMN-Go to Termux.
            intentApp.removeExtra(OMNGO_CAPTURE_OUTPUT);
            // Separate stdout and stderr are captured in background mode
            // only. A capture URI that says nothing else defaults to
            // background, thus capture works with no extra word. The note
            // can still force a visible terminal session with
            // B.com.termux.RUN_COMMAND_BACKGROUND=false.
            if (!intentApp.hasExtra(TERMUX_RUN_COMMAND_BACKGROUND)) {
                intentApp.putExtra(TERMUX_RUN_COMMAND_BACKGROUND, true);
            }
            // Base intent for the own broadcast of OMN-Go. It carries the
            // stream selection and the label, thus handleTermuxResult gets
            // them back beside the result bundle of Termux. Termux adds
            // that bundle into this same intent, and the PendingIntent must
            // therefore be mutable on API 31+.
            android.content.Intent resultIntent = new android.content.Intent(ACTION_TERMUX_RESULT);
            resultIntent.setPackage(a.getPackageName());
            resultIntent.putExtra(OMNGO_STREAM, stream);
            String capLabel = intentApp.getStringExtra(TERMUX_RUN_COMMAND_LABEL);
            resultIntent.putExtra(OMNGO_LABEL, capLabel != null ? capLabel : "");
            int piFlags = android.app.PendingIntent.FLAG_UPDATE_CURRENT;
            if (android.os.Build.VERSION.SDK_INT >= 31) {
                piFlags |= android.app.PendingIntent.FLAG_MUTABLE;
            }
            android.app.PendingIntent resultPI = android.app.PendingIntent.getBroadcast(
                a, termuxResultRequestCounter++, resultIntent, piFlags);
            intentApp.putExtra(TERMUX_RUN_COMMAND_PENDING_INTENT, resultPI);
        }

        // Human-readable summary for the confirmation dialog: the note's
        // RUN_COMMAND_LABEL if present, then the resolved command line.
        String label = intentApp.getStringExtra(TERMUX_RUN_COMMAND_LABEL);
        String path = intentApp.getStringExtra(TERMUX_RUN_COMMAND_PATH);
        String[] args = intentApp.getStringArrayExtra(TERMUX_RUN_COMMAND_ARGS);
        StringBuilder summary = new StringBuilder();
        if (label != null && !label.isEmpty()) {
            summary.append(label).append("\n\n");
        }
        summary.append(path != null ? path : "");
        if (args != null) {
            for (String arg : args) {
                summary.append(' ').append(arg);
            }
        }

        new android.app.AlertDialog.Builder(a)
            .setTitle("Run Termux command?")
            .setMessage(summary.toString().trim())
            .setPositiveButton("Run", (d, w) -> startTermuxService(intentApp))
            .setNegativeButton("Cancel", null)
            .show();
    }

    // Starts com.termux/.app.RunCommandService. On API 26+ the service must
    // be started with startForegroundService(). RunCommandService promotes
    // itself to the foreground with a notification, and a plain
    // startService() there can throw once Termux calls startForeground
    // late. Below 26, use startService(). A SecurityException here almost
    // always means the allow-external-apps setting of Termux is not set,
    // thus the message points the user at it.
    void startTermuxService(final android.content.Intent intentApp) {
        try {
            if (android.os.Build.VERSION.SDK_INT >= 26) {
                a.startForegroundService(intentApp);
            } else {
                a.startService(intentApp);
            }
        } catch (SecurityException e) {
            a.showToast("Termux refused the command. In Termux, set allow-external-apps=true "
                + "in ~/.termux/termux.properties and run termux-reload-settings.");
        } catch (Exception e) {
            e.printStackTrace();
            a.showToast("Couldn't start the Termux command.");
        }
    }

    boolean isPackageInstalled(String pkg) {
        try {
            a.getPackageManager().getPackageInfo(pkg, 0);
            return true;
        } catch (android.content.pm.PackageManager.NameNotFoundException e) {
            return false;
        }
    }
}
