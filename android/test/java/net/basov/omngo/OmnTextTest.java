package net.basov.omngo;

import java.io.ByteArrayInputStream;

// ----------------------------------------------------------------------
// The test of OmnText
// ----------------------------------------------------------------------
//
// OmnText holds the text rules of the Android layer. Each rule was a
// private method of MainActivity, and no test could run it. See the banner
// of OmnText.java.
//
// This file has the same form as OmnConfigTest.java: a main method, its own
// check helpers and no JUnit. See the banner of that file for the reasons.
//
// TestJavaUnitTests in backend/internal/repocheck/java_test.go compiles this
// file together with OmnText.java and OmnConfig.java, and runs it.
//
// This file holds no byte above 127. A character above 127 is an escape.
public final class OmnTextTest {

    private static int failures = 0;

    public static void main(String[] args) throws Exception {
        theNameOfANote();
        aNoteAsText();
        theNameOfASharedFile();
        theWordsOfTheServer();
        theOutputOfACommand();
        theNameOfAnExport();
        theBytesOfAnAnswer();

        if (failures > 0) {
            System.out.println(failures + " check(s) failed");
            System.exit(1);
        }
        System.out.println("OmnTextTest: every check passed");
    }

    // A shared file is a note by its type or by the end of its name. The
    // case of the name does not matter. ".txt" is not a note.
    private static void theNameOfANote() {
        eq("type: markdown", true, OmnText.isMarkdownType("text/markdown"));
        eq("type: x-markdown", true, OmnText.isMarkdownType("text/x-markdown"));
        eq("type: plain", false, OmnText.isMarkdownType("text/plain"));
        eq("type: octet-stream", false, OmnText.isMarkdownType("application/octet-stream"));
        eq("type: none", false, OmnText.isMarkdownType(null));

        eq("name: .md", true, OmnText.hasNoteExtension("Note.md"));
        eq("name: .MD", true, OmnText.hasNoteExtension("NOTE.MD"));
        eq("name: .markdown", true, OmnText.hasNoteExtension("a.b.markdown"));
        eq("name: .txt", false, OmnText.hasNoteExtension("Note.txt"));
        eq("name: md in the middle", false, OmnText.hasNoteExtension("Note.md.txt"));
        eq("name: no dot", false, OmnText.hasNoteExtension("md"));
        eq("name: none", false, OmnText.hasNoteExtension(null));
    }

    // Shared TEXT is a note only when its header block holds a FileName
    // line. Each other text goes to the note box of Quick Notes.
    private static void aNoteAsText() {
        eq("note: with FileName", true,
            OmnText.looksLikeSharedNote("Title: A\nFileName: A.md\n\nbody"));
        eq("note: FileName first", true, OmnText.looksLikeSharedNote("FileName: A.md\n"));
        eq("note: the case of the key", true, OmnText.looksLikeSharedNote("filename : A.md\n"));
        eq("note: line ends of Windows", true,
            OmnText.looksLikeSharedNote("Title: A\r\nFileName: A.md\r\n\r\nbody"));

        eq("text: none", false, OmnText.looksLikeSharedNote(null));
        eq("text: empty", false, OmnText.looksLikeSharedNote(""));
        eq("text: a thought", false, OmnText.looksLikeSharedNote("buy milk"));
        eq("text: a link", false, OmnText.looksLikeSharedNote("https://example.org/page"));
        eq("text: a header with no FileName", false,
            OmnText.looksLikeSharedNote("Title: A\nTags: b\n\nbody"));
        // The header block ends at the first empty line.
        eq("text: FileName in the body", false,
            OmnText.looksLikeSharedNote("Title: A\n\nFileName: A.md\n"));
        // A heading and a line of markup can hold a colon. They are still
        // not a header line.
        eq("text: a heading first", false, OmnText.looksLikeSharedNote("# A: b\nFileName: A.md\n"));
        eq("text: markup first", false,
            OmnText.looksLikeSharedNote("<a href=\"x:y\">\nFileName: A.md\n"));
        eq("text: a space first", false, OmnText.looksLikeSharedNote(" Title: A\nFileName: A.md\n"));
        eq("text: a line with no key", false, OmnText.looksLikeSharedNote("Title: A\nprose\nFileName: A.md\n"));

        // The search reads 32 lines and no more.
        StringBuilder far = new StringBuilder();
        for (int i = 0; i < 32; i++) far.append("K").append(i).append(": v\n");
        eq("text: FileName on line 33", false, OmnText.looksLikeSharedNote(far + "FileName: A.md\n"));
        StringBuilder near = new StringBuilder();
        for (int i = 0; i < 31; i++) near.append("K").append(i).append(": v\n");
        eq("note: FileName on line 32", true, OmnText.looksLikeSharedNote(near + "FileName: A.md\n"));
    }

    // The name of a shared file becomes a path below the storage directory.
    // The result must hold no path separator, and it must have an extension.
    private static void theNameOfASharedFile() {
        eq("file: a plain name", "photo.png", OmnText.sanitizeSharedFilename("photo.png", false));
        eq("file: a path", "c.png", OmnText.sanitizeSharedFilename("a/b/c.png", false));
        eq("file: a path that goes up", "passwd.png",
            OmnText.sanitizeSharedFilename("../../etc/passwd", false));
        eq("file: a path of Windows", "x.json", OmnText.sanitizeSharedFilename("C:\\dir\\x.json", true));
        eq("file: no extension, image", "scan.png", OmnText.sanitizeSharedFilename("scan", false));
        eq("file: no extension, JSON", "data.json", OmnText.sanitizeSharedFilename("data", true));
        eq("file: a hidden name", ".profile.png", OmnText.sanitizeSharedFilename(".profile", false));
        eq("file: a separator at the end", ".json", OmnText.sanitizeSharedFilename("dir/", true));

        // A provider that gives no name gets a name with the time in it.
        String made = OmnText.sanitizeSharedFilename(null, false);
        eq("file: no name, image", true, made.matches("shared_[0-9]+\\.png"));
        made = OmnText.sanitizeSharedFilename("   ", true);
        eq("file: a name of spaces, JSON", true, made.matches("shared_[0-9]+\\.json"));

        for (String name : new String[]{"a/b/c.png", "..\\..\\x.json", "dir/", "../../etc/passwd"}) {
            String out = OmnText.sanitizeSharedFilename(name, false);
            eq("file: no separator in " + name, false, out.indexOf('/') >= 0 || out.indexOf('\\') >= 0);
        }
    }

    // A refusal of the import shows the words of the server. An answer
    // with no message shows the status code.
    private static void theWordsOfTheServer() {
        eq("import: a message", "the note is too large",
            OmnText.importErrorMessage("{\"status\":\"error\",\"message\":\"the note is too large\"}", 413));
        eq("import: an escape", "\u0444\u0430\u0439\u043b \"a\"",
            OmnText.importErrorMessage("{\"message\":\"\\u0444\\u0430\\u0439\\u043b \\\"a\\\"\"}", 400));
        eq("import: an empty message", "the server answered HTTP 500",
            OmnText.importErrorMessage("{\"message\":\"\"}", 500));
        eq("import: no message", "the server answered HTTP 403",
            OmnText.importErrorMessage("{\"status\":\"error\"}", 403));
        eq("import: a message that is a number", "the server answered HTTP 400",
            OmnText.importErrorMessage("{\"message\":7}", 400));
        eq("import: not JSON", "the server answered HTTP 502",
            OmnText.importErrorMessage("<html>Bad Gateway</html>", 502));
        eq("import: an empty body", "the server answered HTTP 500", OmnText.importErrorMessage("", 500));
    }

    // The text that goes to the review dialog after a Termux command.
    private static void theOutputOfACommand() {
        eq("capture: stdout", "out", OmnText.buildCaptureText("stdout", "out\n", "err\n", 0));
        eq("capture: stderr", "err", OmnText.buildCaptureText("stderr", "out\n", "err\n", 0));
        eq("capture: both", "out\n\nerr", OmnText.buildCaptureText("both", "out\n", "err\n", 0));
        eq("capture: both, no stderr", "out", OmnText.buildCaptureText("both", "out", "", 0));
        eq("capture: both, no stdout", "err", OmnText.buildCaptureText("both", "", "err", 0));
        eq("capture: an unknown stream is stdout", "out", OmnText.buildCaptureText("other", "out", "err", 0));
        eq("capture: no stream is stdout", "out", OmnText.buildCaptureText(null, "out", "err", 0));
        eq("capture: null values", "", OmnText.buildCaptureText("both", null, null, 0));

        // A failed command shows its exit code. A good one does not.
        eq("capture: a failure", "out\nexit code: 2", OmnText.buildCaptureText("stdout", "out", "", 2));
        eq("capture: a failure with no output", "exit code: 127",
            OmnText.buildCaptureText("stdout", "", "not found", 127));
    }

    // The name of the export becomes a path below the cache directory. It
    // keeps letters, digits, the dot, the underscore and the hyphen.
    private static void theNameOfAnExport() {
        eq("export: a plain name", "Note.md",
            OmnText.exportFilename("attachment; filename=\"Note.md\""));
        eq("export: a path", "etcpasswd.md",
            OmnText.exportFilename("attachment; filename=\"../../etc/passwd\""));
        eq("export: a hidden name", "profile.md",
            OmnText.exportFilename("attachment; filename=\"..profile\""));
        eq("export: a space and a letter above 127", "MyNote.md",
            OmnText.exportFilename("attachment; filename=\"My \u00e9Note.md\""));
        eq("export: no extension", "Note.md", OmnText.exportFilename("attachment; filename=\"Note\""));
        eq("export: the case of the extension", "Note.MD",
            OmnText.exportFilename("attachment; filename=\"Note.MD\""));
        eq("export: a second extension", "Note.txt.md",
            OmnText.exportFilename("attachment; filename=\"Note.txt\""));
        eq("export: no header", "note.md", OmnText.exportFilename(null));
        eq("export: no file name", "note.md", OmnText.exportFilename("attachment"));
        eq("export: an empty name", "note.md", OmnText.exportFilename("attachment; filename=\"\""));
        eq("export: no closing quote", "note.md", OmnText.exportFilename("attachment; filename=\"Note.md"));
        eq("export: only dots", "note.md", OmnText.exportFilename("attachment; filename=\"...\""));
    }

    // readAllUtf8 reads each byte of a stream, also above one buffer.
    private static void theBytesOfAnAnswer() throws Exception {
        eq("read: empty", "", OmnText.readAllUtf8(new ByteArrayInputStream(new byte[0])));
        String text = "\u043f\u0440\u0438\u0432\u0435\u0442, caf\u00e9";
        eq("read: UTF-8", text, OmnText.readAllUtf8(new ByteArrayInputStream(text.getBytes("UTF-8"))));

        StringBuilder big = new StringBuilder();
        for (int i = 0; i < 5000; i++) big.append("line ").append(i).append('\n');
        eq("read: more than one buffer", big.toString(),
            OmnText.readAllUtf8(new ByteArrayInputStream(big.toString().getBytes("UTF-8"))));
    }

    // ------------------------------------------------------------------
    // The check helpers
    // ------------------------------------------------------------------

    private static void eq(String what, Object want, Object got) {
        if (want == null ? got == null : want.equals(got)) return;
        failures++;
        System.out.println("FAIL " + what + ": got " + show(got) + ", want " + show(want));
    }

    private static void eq(String what, boolean want, boolean got) {
        eq(what, Boolean.valueOf(want), Boolean.valueOf(got));
    }

    private static String show(Object o) {
        if (o == null) return "null";
        return "\"" + o.toString().replace("\n", "\\n") + "\"";
    }
}
