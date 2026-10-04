package net.basov.omngo;

import java.io.ByteArrayOutputStream;
import java.io.IOException;
import java.io.InputStream;
import java.util.Arrays;
import java.util.HashSet;
import java.util.Locale;
import java.util.Set;

// ----------------------------------------------------------------------
// The text rules of the Android layer
// ----------------------------------------------------------------------
//
// Each method here takes text and answers text or a yes. None of them
// touches a window, an intent or a file of the device.
//
// WHY THEY ARE IN ONE CLASS OF THEIR OWN. They were private methods of
// MainActivity, and nothing outside an emulator could run them. This class
// imports no Android package, thus a plain JVM loads it.
// android/test/java/net/basov/omngo/OmnTextTest.java runs each rule, and
// TestJavaUnitTests in backend/internal/repocheck/java_test.go runs that
// file in the gate. OmnConfig is the same pattern.
//
// KEEP IT THAT WAY. An import of android.* or org.json makes the test fail
// to compile. TestPureJavaClassesImportNoAndroidPackage names that rule.
//
// This file holds no byte above 127, because a raw javac reads it. See
// TestCompiledJavaSourcesAreASCII.
final class OmnText {

    private OmnText() {
    }

    /** Names a shared note by extension. ".txt" is not one: v1 sends notes. */
    static final Set<String> SHARED_NOTE_EXT =
        new HashSet<>(Arrays.asList(".md", ".markdown"));

    static boolean isMarkdownType(String type) {
        return "text/markdown".equals(type) || "text/x-markdown".equals(type);
    }

    static boolean hasNoteExtension(String name) {
        if (name == null) return false;
        String lower = name.toLowerCase(Locale.ROOT);
        int dot = lower.lastIndexOf('.');
        return dot >= 0 && SHARED_NOTE_EXT.contains(lower.substring(dot));
    }

    /**
     * Whether shared TEXT is a note rather than a thought or a link.
     *
     * A ROUTING hint and nothing more: it decides which of two paths the
     * text takes, and the server validates whatever arrives. The rule is the
     * one splitFrontMatter uses in Go - a header block of "Key: value" lines
     * that does not begin with a space, "#" or "<" - and the question asked
     * of it is whether that block carries a FileName: line.
     *
     * Without this a note sent as text would land in the note box as a
     * quick note, header block and all.
     */
    static boolean looksLikeSharedNote(String text) {
        if (text == null || text.isEmpty()) return false;
        String[] lines = text.split("\n");
        for (int i = 0; i < lines.length && i < 32; i++) {
            String line = lines[i];
            if (line.endsWith("\r")) {
                line = line.substring(0, line.length() - 1);
            }
            if (line.trim().isEmpty()) return false;          // the block ended
            if (line.startsWith(" ") || line.startsWith("#") || line.startsWith("<")) return false;
            int colon = line.indexOf(':');
            if (colon <= 0) return false;                      // not a header line
            if ("filename".equals(line.substring(0, colon).trim()
                    .toLowerCase(Locale.ROOT))) {
                return true;
            }
        }
        return false;
    }

    /** One tree of files that the user uploads or shares. */
    static final class UserFileTree {
        /** The route of the server that takes a file of the tree. */
        final String upload;
        /** The name of one file of the tree in a message. */
        final String word;
        /** The extensions that the tree takes. */
        final String[] exts;
        /** The types that a sender can give such a file. */
        final String[] types;

        UserFileTree(String upload, String word, String[] exts, String[] types) {
            this.upload = upload;
            this.word = word;
            this.exts = exts;
            this.types = types;
        }

        boolean hasExtension(String ext) {
            return Arrays.asList(exts).contains(ext);
        }
    }

    // USER_FILE_TREES is the copy of config.UserFileTrees in
    // backend/internal/config/user_files.go. The share path must know which
    // file to take before the Go server sees it, thus it cannot read that
    // table. TestUserFileTreesHaveTheirCopies in backend/internal/repocheck
    // compares the rows with the Go table.
    static final UserFileTree[] USER_FILE_TREES = {
        new UserFileTree("/api/upload_json", "JSON file",
            new String[]{".json", ".jsonl"},
            new String[]{"application/json", "application/jsonl"}),
        new UserFileTree("/api/upload_contacts", "Contact file",
            new String[]{".vcf"},
            new String[]{"text/vcard", "text/x-vcard"}),
        new UserFileTree("/api/upload_calendars", "Calendar file",
            new String[]{".ics", ".vcs"},
            new String[]{"text/calendar", "text/x-vcalendar"}),
    };

    // Answers the tree of a shared file, or null for each other file. The
    // extension of the name decides before the declared type. Many senders
    // give a generic type or a wrong one.
    static UserFileTree userFileTree(String mimeType, String displayName) {
        String ext = "";
        if (displayName != null) {
            String lower = displayName.toLowerCase(Locale.ROOT);
            int dot = lower.lastIndexOf('.');
            if (dot >= 0) ext = lower.substring(dot);
        }
        for (UserFileTree tree : USER_FILE_TREES) {
            if (tree.hasExtension(ext)) return tree;
        }
        if (mimeType == null) return null;
        for (UserFileTree tree : USER_FILE_TREES) {
            if (Arrays.asList(tree.types).contains(mimeType)) return tree;
        }
        return null;
    }

    // Falls back to a generated name when the content provider supplies
    // none. It also strips each path separator that a provider can smuggle
    // into DISPLAY_NAME, thus this can never write outside destDir.
    // defaultExt is the extension of a name that has none.
    static String sanitizeSharedFilename(String displayName, String defaultExt) {
        String name = displayName;
        if (name == null || name.trim().isEmpty()) {
            name = "shared_" + System.currentTimeMillis() + defaultExt;
        }
        name = name.replace('\\', '/');
        int slash = name.lastIndexOf('/');
        if (slash >= 0) name = name.substring(slash + 1);
        if (name.isEmpty() || name.lastIndexOf('.') <= 0) {
            name = name + defaultExt;
        }
        return name;
    }

    /** The server's own words when it refuses, rather than a status code. */
    static String importErrorMessage(String body, int code) {
        // OmnConfig.parseFlat and not org.json, which a plain JVM cannot
        // load. A body that is not JSON gives an empty map.
        Object message = OmnConfig.parseFlat(body).get("message");
        if (message instanceof String && !((String) message).isEmpty()) {
            return (String) message;
        }
        return "the server answered HTTP " + code;
    }

    // Assembles the text to paste from a Termux result according to the
    // requested stream ("stdout" default / "stderr" / "both"). An exit-code
    // line is appended only when the command failed (non-zero), so a normal
    // success stays clean.
    static String buildCaptureText(String stream, String stdout, String stderr, int exitCode) {
        if (stdout == null) stdout = "";
        if (stderr == null) stderr = "";
        StringBuilder sb = new StringBuilder();
        if ("stderr".equals(stream)) {
            sb.append(stderr);
        } else if ("both".equals(stream)) {
            sb.append(stdout);
            if (!stderr.isEmpty()) {
                if (sb.length() > 0) sb.append('\n');
                sb.append(stderr);
            }
        } else { // "stdout" (default)
            sb.append(stdout);
        }
        if (exitCode != 0) {
            if (sb.length() > 0) sb.append('\n');
            sb.append("exit code: ").append(exitCode);
        }
        return sb.toString().trim();
    }

    /**
     * The file name for the export, from the server's Content-Disposition.
     *
     * Sanitized even though OMN-Go's own server wrote it: this string becomes
     * a path under the cache directory, and a name that arrives over a socket
     * is not a name to join onto a path untouched. flattenExportName in
     * backend/internal/exchange/exchange.go already restricts it to the same
     * set, so a well-formed answer passes through unchanged.
     */
    static String exportFilename(String contentDisposition) {
        String raw = "";
        if (contentDisposition != null) {
            int at = contentDisposition.indexOf("filename=\"");
            if (at >= 0) {
                int end = contentDisposition.indexOf('"', at + 10);
                if (end > at) {
                    raw = contentDisposition.substring(at + 10, end);
                }
            }
        }
        StringBuilder kept = new StringBuilder();
        for (int i = 0; i < raw.length(); i++) {
            char c = raw.charAt(i);
            boolean ok = (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
                    || (c >= '0' && c <= '9') || c == '.' || c == '_' || c == '-';
            if (ok) {
                kept.append(c);
            }
        }
        String name = kept.toString();
        while (name.startsWith(".")) {
            name = name.substring(1);
        }
        if (name.isEmpty()) {
            name = "note.md";
        }
        if (!name.toLowerCase(Locale.ROOT).endsWith(".md")) {
            name = name + ".md";
        }
        return name;
    }

    /** Reads a whole response as UTF-8. A note is small enough to hold. */
    static String readAllUtf8(InputStream in) throws IOException {
        ByteArrayOutputStream buffer = new ByteArrayOutputStream();
        byte[] chunk = new byte[8192];
        int read;
        while ((read = in.read(chunk)) != -1) {
            buffer.write(chunk, 0, read);
        }
        return new String(buffer.toByteArray(), "UTF-8");
    }
}
