package net.basov.omngo;

// ShareIn takes what another application gives to OMN-Go: a note, an image,
// a JSON file, a contact or a calendar. MainActivity reads the intent and
// calls a method here.
// See onCreate and onNewIntent of MainActivity.
//
// The text rules are in OmnText, where a plain JVM can test them.
//
// A member here is not private. An inner class reads most of them, and the
// compiler adds a bridge method for each private member that an inner
// class reads.
final class ShareIn {
    final MainActivity a;

    ShareIn(MainActivity activity) {
        a = activity;
    }

    // ----------------------------------------------------------------------
    // Shared file handling (images / JSON via Android's "Share to" chooser)
    // ----------------------------------------------------------------------
    //
    // Shared TEXT is handled with window.handleShare in JS. See the
    // text/plain branches of MainActivity. That works whatever the app state is,
    // because it always targets a page-independent modal, which is Quick
    // Note or Bookmark. Such a modal exists on every ordinary view page.
    //
    // A shared FILE has no equivalent safe target. The WebView cannot read
    // a content:// Uri without a JS bridge. There is also no guarantee that
    // the app shows a page that defines window.handleShare. It could be on
    // editor.html, mid-edit of an unrelated note, which loads no
    // omn-go-core.js at all.
    //
    // So this is handled entirely natively, and it is independent of
    // whatever the WebView does.
    //   1. Validate the shared file and copy it straight onto the same
    //      on-disk tree that the Go server serves from, which is
    //      storageDir()/html/images or .../user_json. A contact and a
    //      calendar have a tree each. See OmnText.USER_FILE_TREES. It
    //      enforces the same
    //      extension whitelist and max-size limit that saveUploadedFile
    //      enforces on the server for the own drag-and-drop upload of the
    //      editor. The limit comes from max_upload_size_mb in config.json.
    //      See backend/internal/app/upload_handlers.go. If
    //      imageUploadExtensions changes there, change the whitelist here
    //      too. OmnText.USER_FILE_TREES holds each other list.
    //   2. Build the same snippet format that those Go handlers return.
    //      An image gets an HTML <img class="omn-imported-image"> tag. A
    //      JSON file, a contact and a calendar get a markdown link, for
    //      example [name](/user_json/name). POST it
    //      as a Quick Note with the existing /api/quick endpoint. The
    //      QuickNotes.md append and compile logic of the server is thus
    //      reused and not duplicated here. See handleQuickNote. A loopback
    //      request bypasses authMiddleware entirely, see
    //      backend/internal/app/middleware.go, thus no session or cookie
    //      handling is needed.
    // This runs entirely on a background thread and never touches webView,
    // thus it is safe whatever page is loaded. Only a single-file share is
    // handled, which is ACTION_SEND and not ACTION_SEND_MULTIPLE. That
    // matches the scope of the text/plain share handling of MainActivity.

    // The image extensions that this app accepts through a share. Keep the
    // set the same as imageUploadExtensions in
    // backend/internal/app/upload_handlers.go. This set is the one source
    // in this file. isSharedFileIntent and handleSharedFile both use it.
    // OmnText.USER_FILE_TREES holds the extensions of a JSON file, of a
    // contact and of a calendar.
    static final java.util.Set<String> SHARED_IMAGE_EXT =
        new java.util.HashSet<>(java.util.Arrays.asList(".png", ".jpg", ".jpeg", ".gif", ".webp", ".svg"));

    // ----------------------------------------------------------------------
    // Receiving a note (note exchange, phase 5)
    // ----------------------------------------------------------------------
    //
    // A note arrives from Telegram, a mail client, LocalSend or a file
    // manager. It comes as a FILE through the share sheet, as a FILE
    // through ACTION_VIEW, or as TEXT in the message body.
    //
    // Every one of them ends at the same place, POST /api/import/note. The
    // rules live in Go, in backend/internal/exchange/exchange.go. Those rules
    // say where a note lands, what its name becomes and how a collision is
    // numbered. This side repeats none of them. The block comment above
    // handleSharedFile below documents that trap, where the native image path
    // had to be kept in step with handleUpload by hand.

    /**
     * A shared or opened FILE that is a note.
     *
     * Checked before isSharedFileIntent, which owns images and JSON. The
     * declared type is not trusted on its own: Telegram and several mail
     * clients label a .md attachment "application/octet-stream", which is
     * why that type is in the manifest at all - so the file's own name
     * decides when the type says nothing useful.
     */
    boolean isSharedNoteIntent(android.content.Intent intent) {
        return sharedNoteUri(intent) != null;
    }

    /** The URI of a shared or opened note, or null. */
    android.net.Uri sharedNoteUri(android.content.Intent intent) {
        String action = intent.getAction();
        android.net.Uri uri = null;
        if (android.content.Intent.ACTION_SEND.equals(action)) {
            uri = intent.getParcelableExtra(android.content.Intent.EXTRA_STREAM);
        } else if (android.content.Intent.ACTION_VIEW.equals(action)) {
            uri = intent.getData();
        }
        if (uri == null) return null;
        if (OmnText.isMarkdownType(intent.getType())) return uri;
        return OmnText.hasNoteExtension(queryDisplayName(uri)) ? uri : null;
    }

    /** Reads a shared file and imports it. */
    void importSharedNote(final android.net.Uri uri) {
        new Thread(new Runnable() {
            @Override
            public void run() {
                try {
                    String displayName = queryDisplayName(uri);
                    long limit = (long) a.readMaxUploadSizeMB() * 1024 * 1024;
                    byte[] body = readUriCapped(uri, limit);
                    if (body == null) {
                        a.showToast("Not imported: the note is larger than the upload limit.");
                        return;
                    }
                    postImportedNote(body, displayName);
                } catch (Exception e) {
                    e.printStackTrace();
                    a.showToast("Could not import the note: " + e.getMessage());
                }
            }
        }).start();
    }

    /** Imports shared TEXT that looksLikeSharedNote accepted. */
    void importSharedText(final String text) {
        new Thread(new Runnable() {
            @Override
            public void run() {
                try {
                    postImportedNote(text.getBytes("UTF-8"), null);
                } catch (Exception e) {
                    e.printStackTrace();
                    a.showToast("Could not import the note: " + e.getMessage());
                }
            }
        }).start();
    }

    /**
     * POSTs the note to the server and acts on the answer.
     *
     * The endpoint is admin only and this request comes from 127.0.0.1,
     * which bypasses that - the device is not a remote caller to its own
     * server. The retry mirrors postQuickNoteWithRetry: a share can arrive
     * during a cold start, while the Go side is still coming up.
     */
    void postImportedNote(byte[] body, String displayName) throws java.io.IOException {
        String answer;
        try {
            answer = postImport(body, displayName);
        } catch (java.io.IOException first) {
            try {
                Thread.sleep(1500);
            } catch (InterruptedException ignored) {
                Thread.currentThread().interrupt();
            }
            answer = postImport(body, displayName);
        }

        String url = null;
        String base = null;
        try {
            org.json.JSONObject json = new org.json.JSONObject(answer);
            url = json.optString("url", null);
            base = json.optString("base", null);
        } catch (org.json.JSONException e) {
            // A 200 that is not JSON should not happen, and is not worth
            // losing the import over: the note is written either way.
            e.printStackTrace();
        }

        a.showToast(base != null ? ("Note imported: " + base) : "Note imported.");
        if (url != null && !url.isEmpty()) {
            openImportedNote(url);
        }
    }

    String postImport(byte[] body, String displayName) throws java.io.IOException {
        String target = a.serverBase() + "/api/import/note";
        if (displayName != null && !displayName.isEmpty()) {
            target += "?name=" + java.net.URLEncoder.encode(displayName, "UTF-8");
        }
        java.net.HttpURLConnection conn =
            (java.net.HttpURLConnection) new java.net.URL(target).openConnection();
        try {
            conn.setRequestMethod("POST");
            conn.setDoOutput(true);
            conn.setRequestProperty("Content-Type", "text/markdown; charset=utf-8");
            conn.setFixedLengthStreamingMode(body.length);
            java.io.OutputStream os = conn.getOutputStream();
            os.write(body);
            os.close();

            int code = conn.getResponseCode();
            java.io.InputStream in = (code >= 400) ? conn.getErrorStream() : conn.getInputStream();
            String text = in == null ? "" : OmnText.readAllUtf8(in);
            if (code != 200) {
                throw new java.io.IOException(OmnText.importErrorMessage(text, code));
            }
            return text;
        } finally {
            conn.disconnect();
        }
    }

    /**
     * Opens the note that just arrived - EXCEPT when the editor is open.
     *
     * A share arrives while the user is in the middle of something. The
     * editor page holds text that is not saved, and a note that came from
     * Telegram is not worth throwing it away. The toast has already named
     * the note, and the incoming index lists it when the user is ready.
     *
     * The editor is recognized by its URL: ?edit=true is served by the
     * standalone editor page (see serveEditor in
     * backend/internal/app/handlers.go), and the query stays in the address.
     */
    void openImportedNote(final String url) {
        a.runOnUiThread(new Runnable() {
            @Override
            public void run() {
                if (a.webView == null) return;
                String current = a.webView.getUrl();
                if (current != null && current.contains("edit=true")) return;
                a.webView.loadUrl(a.serverBase() + url);
            }
        });
    }

    /**
     * Reads a content:// URI into memory, or null when it is over the limit.
     *
     * Refuses rather than truncates: half a note is not an import. This is
     * the same choice readImportBody makes on the Go side, and the reason
     * neither of them is readCapped of backend/internal/search/search.go.
     */
    byte[] readUriCapped(android.net.Uri uri, long maxBytes) throws java.io.IOException {
        java.io.InputStream in = a.getContentResolver().openInputStream(uri);
        if (in == null) throw new java.io.IOException("cannot open the shared file");
        try {
            java.io.ByteArrayOutputStream out = new java.io.ByteArrayOutputStream();
            byte[] chunk = new byte[8192];
            long total = 0;
            int read;
            while ((read = in.read(chunk)) != -1) {
                total += read;
                if (maxBytes > 0 && total > maxBytes) return null;
                out.write(chunk, 0, read);
            }
            return out.toByteArray();
        } finally {
            in.close();
        }
    }

    boolean isSharedFileIntent(android.content.Intent intent) {
        if (!android.content.Intent.ACTION_SEND.equals(intent.getAction())) return false;
        android.net.Uri stream = intent.getParcelableExtra(android.content.Intent.EXTRA_STREAM);
        if (stream == null) return false;
        String type = intent.getType();
        if (type != null && (type.startsWith("image/") || OmnText.userFileTree(type, null) != null)) {
            return true;
        }
        // Many senders hand a JSON share over with a generic or wrong MIME
        // type. File managers, chat apps and "Files" do this, and an image
        // share sometimes as well. The type is application/octet-stream,
        // text/plain, or no type at all, and not "application/json" or
        // "image/*". That is exactly why JSON sharing "did nothing" in
        // practice. The type check above never matched, thus
        // isSharedFileIntent returned false and the whole share was
        // silently dropped, and the file itself was perfectly fine. Fall
        // back to the own display name and extension of the shared file,
        // and do not trust the declared type.
        String name = queryDisplayName(stream);
        if (name != null) {
            if (OmnText.userFileTree(null, name) != null) return true;
            String lower = name.toLowerCase(java.util.Locale.ROOT);
            int dot = lower.lastIndexOf('.');
            if (dot >= 0 && SHARED_IMAGE_EXT.contains(lower.substring(dot))) {
                return true;
            }
        }
        return false;
    }

    void handleSharedFile(final android.net.Uri uri, final String mimeType) {
        new Thread(new Runnable() {
            @Override
            public void run() {
                try {
                    String displayName = queryDisplayName(uri);
                    // tree is null for an image.
                    OmnText.UserFileTree tree = OmnText.userFileTree(mimeType, displayName);

                    String filename = OmnText.sanitizeSharedFilename(displayName, tree != null ? tree.exts[0] : ".png");
                    String ext = filename.substring(filename.lastIndexOf('.')).toLowerCase(java.util.Locale.ROOT);
                    boolean allowed = tree != null ? tree.hasExtension(ext) : SHARED_IMAGE_EXT.contains(ext);
                    if (!allowed) {
                        a.showToast("Not saved: only images, .json, .jsonl, .vcf, .ics or .vcs files can be shared into OMN-Go.");
                        return;
                    }

                    long maxBytes = (long) a.readMaxUploadSizeMB() * 1024 * 1024;

                    String subDir = tree != null ? tree.dir : "images";
                    java.io.File destDir = new java.io.File(a.storageDir() + "/html/" + subDir);
                    destDir.mkdirs();
                    java.io.File destFile = new java.io.File(destDir, filename);

                    long copied = copyUriToFile(uri, destFile, maxBytes);
                    if (copied < 0) {
                        destFile.delete();
                        a.showToast("Not saved: file is larger than the configured upload limit.");
                        return;
                    }

                    // This is the format that handleUpload and
                    // handleUploadUserFile in
                    // backend/internal/app/upload_handlers.go make.
                    // If either one changes, change this code by hand.
                    //
                    // Images went from markdown image syntax to an HTML
                    // <img> tag, with the .omn-imported-image class. See
                    // omn-go-core.css. A dropped image thus gets a sensible
                    // default size, and it does not render at full native
                    // resolution. This native share path builds its own
                    // snippet, independent of the Go server, see the block
                    // comment above. It still emitted the old markdown form
                    // here. An image shared into a fresh Android install
                    // then rendered without the class that desktop
                    // drag-and-drop already got.
                    //
                    // A file of a tree gets a markdown link. See
                    // OmnText.userFileLink.
                    String snippet;
                    if (tree != null) {
                        snippet = OmnText.userFileLink(tree.dir, filename);
                    } else {
                        String escapedName = android.text.Html.escapeHtml(filename);
                        snippet = "\n<img src=\"/images/" + escapedName + "\" alt=\"" + escapedName
                            + "\" class=\"omn-imported-image\" />\n";
                    }

                    postQuickNoteWithRetry(snippet);
                    a.showToast((tree != null ? tree.word : "Image") + " added to Quick Notes");
                } catch (Exception e) {
                    e.printStackTrace();
                    a.showToast("Failed to save shared file: " + e.getMessage());
                }
            }
        }).start();
    }

    String queryDisplayName(android.net.Uri uri) {
        String name = null;
        android.database.Cursor cursor = a.getContentResolver().query(uri, null, null, null, null);
        if (cursor != null) {
            try {
                int idx = cursor.getColumnIndex(android.provider.OpenableColumns.DISPLAY_NAME);
                if (idx >= 0 && cursor.moveToFirst()) {
                    name = cursor.getString(idx);
                }
            } finally {
                cursor.close();
            }
        }
        return name;
    }

    // Copies the bytes of uri to destFile. It aborts once the stream goes
    // over maxBytes, and it then returns -1 and leaves the partial file
    // for the caller to delete. There is no multipart header with a
    // declared size here, unlike saveUploadedFile on the server. The limit
    // is thus enforced during the stream, and not checked up front.
    long copyUriToFile(android.net.Uri uri, java.io.File destFile, long maxBytes) throws java.io.IOException {
        java.io.InputStream in = a.getContentResolver().openInputStream(uri);
        if (in == null) throw new java.io.IOException("could not open shared file");
        try {
            // Opening the destination is inside this try/finally too, so a
            // FileOutputStream failure (e.g. permissions) still closes
            // `in` instead of leaking it.
            java.io.OutputStream out = new java.io.FileOutputStream(destFile);
            try {
                long total = 0;
                byte[] buf = new byte[8192];
                int n;
                while ((n = in.read(buf)) != -1) {
                    total += n;
                    if (total > maxBytes) {
                        return -1;
                    }
                    out.write(buf, 0, n);
                }
                return total;
            } finally {
                out.close();
            }
        } finally {
            in.close();
        }
    }

    void postQuickNoteWithRetry(String note) throws java.io.IOException {
        try {
            postQuickNote(note);
        } catch (java.io.IOException firstErr) {
            // The Go server may still be in its start. That is the same
            // race that the 1s postDelayed in MainActivity.onCreate already
            // covers. One short retry covers a cold start that is a little
            // slower than usual, and the note is not lost.
            try {
                Thread.sleep(1500);
            } catch (InterruptedException ignored) {
                Thread.currentThread().interrupt();
            }
            postQuickNote(note);
        }
    }

    // POSTs note (already-built markdown) to /api/quick, appending it to
    // QuickNotes.md - see handleQuickNote in
    // backend/internal/app/note_handlers.go.
    void postQuickNote(String note) throws java.io.IOException {
        java.net.URL url = new java.net.URL(a.serverBase() + "/api/quick");
        java.net.HttpURLConnection conn = (java.net.HttpURLConnection) url.openConnection();
        try {
            conn.setRequestMethod("POST");
            conn.setDoOutput(true);
            conn.setRequestProperty("Content-Type", "application/x-www-form-urlencoded; charset=utf-8");
            String body = "note=" + java.net.URLEncoder.encode(note, "UTF-8");
            byte[] bodyBytes = body.getBytes("UTF-8");
            conn.setFixedLengthStreamingMode(bodyBytes.length);
            java.io.OutputStream os = conn.getOutputStream();
            os.write(bodyBytes);
            os.close();
            int code = conn.getResponseCode();
            if (code != 200) {
                throw new java.io.IOException("server returned HTTP " + code);
            }
        } finally {
            conn.disconnect();
        }
    }
}
