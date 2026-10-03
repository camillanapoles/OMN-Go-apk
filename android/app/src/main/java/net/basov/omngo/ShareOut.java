package net.basov.omngo;

// ShareOut sends a note to another application through the share sheet.
// WebViewSetup calls handleShareOut for an "omngo://share" address.
//
// A member here is not private. See the banner of ShareIn for the reason.
final class ShareOut {
    final MainActivity a;

    ShareOut(MainActivity activity) {
        a = activity;
    }

    // ----------------------------------------------------------------------
    // Sending a note out (note exchange, phase 4)
    // ----------------------------------------------------------------------
    //
    // The share sheet is the whole feature. It reaches Telegram, e-mail,
    // LocalSend, Bluetooth, Nearby Share and every other application on the
    // device, so OMN-Go integrates with none of them by name.
    //
    // The bytes come from the SERVER, and not from the note file.
    // /api/export/note adds the "FileName:" line that carries the path of the
    // note. That rule lives in Go, thus the desktop and this side cannot
    // disagree about what a sent note looks like. See
    // backend/internal/exchange/exchange.go. The endpoint is admin-only, and a
    // local connection bypasses that. This request comes from 127.0.0.1, which
    // IS the device.

    /**
     * Answers "omngo://share?name=<note>" and "…&as=text".
     *
     * The work is on a thread because it is an HTTP request; the chooser goes
     * back to the UI thread in startShareChooser.
     */
    void handleShareOut(final android.net.Uri request) {
        final String note = request.getQueryParameter("name");
        final boolean asText = "text".equals(request.getQueryParameter("as"));
        if (note == null || note.isEmpty()) {
            a.showToast("No note to send.");
            return;
        }

        new Thread(new Runnable() {
            @Override
            public void run() {
                try {
                    java.net.URL url = new java.net.URL(a.serverBase() + "/api/export/note?name="
                            + java.net.URLEncoder.encode(note, "UTF-8"));
                    java.net.HttpURLConnection conn =
                            (java.net.HttpURLConnection) url.openConnection();
                    String body;
                    String filename;
                    String description;
                    try {
                        conn.setRequestMethod("GET");
                        int code = conn.getResponseCode();
                        if (code != 200) {
                            throw new java.io.IOException("the server answered HTTP " + code);
                        }
                        filename = OmnText.exportFilename(conn.getHeaderField("Content-Disposition"));
                        description = exportDescription(conn.getHeaderField("X-OMN-Description"));
                        body = OmnText.readAllUtf8(conn.getInputStream());
                    } finally {
                        conn.disconnect();
                    }

                    if (asText) {
                        android.content.Intent send =
                                new android.content.Intent(android.content.Intent.ACTION_SEND);
                        send.setType("text/plain");
                        // No description extra here: a text send carries the
                        // whole note, and the description block is inside it.
                        send.putExtra(android.content.Intent.EXTRA_TEXT, body);
                        send.putExtra(android.content.Intent.EXTRA_SUBJECT, note);
                        startShareChooser(send, "Send note as text");
                        return;
                    }
                    startShareChooser(shareFileIntent(filename, body, description), "Send note");
                } catch (Exception e) {
                    e.printStackTrace();
                    a.showToast("Could not send the note: " + e.getMessage());
                }
            }
        }).start();
    }

    /**
     * Writes the note into the export cache and builds the ACTION_SEND for it.
     *
     * The read grant is what lets the chosen application open the URI:
     * ExportProvider is exported="false", so nothing reaches it without one.
     * ClipData carries the same URI because several applications take the
     * grant from there rather than from EXTRA_STREAM.
     */
    android.content.Intent shareFileIntent(String filename, String body, String description)
            throws java.io.IOException {
        java.io.File dir = ExportProvider.exportDir(a);

        // Sweep by age, and not by "everything but this one". A share that
        // is still in flight has not been read yet. Some applications open
        // the URI when the user presses Send, and not when the sheet
        // appears. A delete of its file would then send an empty
        // attachment. An hour is long after any of them has finished, and
        // it is short enough that the cache cannot grow without bound.
        java.io.File[] previous = dir.listFiles();
        if (previous != null) {
            long cutoff = System.currentTimeMillis() - 60L * 60L * 1000L;
            for (java.io.File old : previous) {
                if (old.lastModified() < cutoff) {
                    old.delete();
                }
            }
        }

        java.io.File out = new java.io.File(dir, filename);
        java.io.OutputStream os = new java.io.FileOutputStream(out);
        try {
            os.write(body.getBytes("UTF-8"));
        } finally {
            os.close();
        }

        android.net.Uri uri = ExportProvider.uriFor(a, out);
        android.content.Intent send =
                new android.content.Intent(android.content.Intent.ACTION_SEND);
        send.setType("text/markdown");
        send.putExtra(android.content.Intent.EXTRA_STREAM, uri);
        send.putExtra(android.content.Intent.EXTRA_SUBJECT, filename);
        // The description block of the note, as the MESSAGE that goes with
        // the file. Telegram makes it the caption, and a mail client makes
        // it the body. An application that has no place for text beside a
        // file ignores the extra. This is thus safe to send to every target
        // in the sheet, and not to a chosen few.
        //
        // Only when the note HAS one. An empty EXTRA_TEXT is not the same
        // as no EXTRA_TEXT. Some clients open an empty message body and
        // wait, where they would attach and send without the extra.
        if (description != null && !description.isEmpty()) {
            send.putExtra(android.content.Intent.EXTRA_TEXT, description);
        }
        send.addFlags(android.content.Intent.FLAG_GRANT_READ_URI_PERMISSION);
        send.setClipData(android.content.ClipData.newUri(a.getContentResolver(), filename, uri));
        return send;
    }

    /**
     * Decodes the X-OMN-Description header (see headerDescription in
     * backend/internal/exchange/exchange.go).
     *
     * Base64 of UTF-8, because the description is a paragraph: it can hold a
     * newline, which would end the header field, and it can hold Cyrillic or
     * an accented letter, which an HTTP header field cannot carry as it
     * stands.
     *
     * A damaged or absent header is not an error. The note goes without a
     * message rather than not at all.
     */
    String exportDescription(String header) {
        if (header == null || header.isEmpty()) {
            return "";
        }
        try {
            byte[] raw = android.util.Base64.decode(header, android.util.Base64.DEFAULT);
            return new String(raw, "UTF-8");
        } catch (Exception e) {
            e.printStackTrace();
            return "";
        }
    }

    /** Shows the chooser. Called from a worker thread, so it hops back. */
    void startShareChooser(final android.content.Intent send, final String title) {
        a.runOnUiThread(new Runnable() {
            @Override
            public void run() {
                try {
                    android.content.Intent chooser =
                            android.content.Intent.createChooser(send, title);
                    // The chooser is the activity that starts next, so the
                    // grant has to be on it as well as on the intent inside.
                    chooser.addFlags(android.content.Intent.FLAG_GRANT_READ_URI_PERMISSION);
                    a.startActivity(chooser);
                } catch (Exception e) {
                    e.printStackTrace();
                    a.showToast("No application on this device can send a note.");
                }
            }
        });
    }
}
