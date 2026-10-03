# 0022. Divide MainActivity into plain classes

* Status: accepted
* Version: 26.10.20
* Code: in `android/app/src/main/java/net/basov/omngo/` the files
  `MainActivity.java`, `WebViewSetup.java`, `Fullscreen.java`,
  `ShareIn.java`, `ShareOut.java`, `IntentBridge.java`, `Shortcuts.java`
  and `OmnText.java`

## Context

`MainActivity.java` held 2243 lines. It held the life cycle, the WebView,
the system bars and each kind of share. It also held the intent links with
Termux and the shortcuts of the home screen. A reader who looked for one
task read past each other task.

The file also held the text rules of those tasks as private methods. An
example is the rule that makes a safe file name from the name of a shared
file. No test could run such a rule, because a test of `MainActivity`
needs an emulator.

## Decision

* `MainActivity` keeps the methods of the life cycle and the code that
  reads an intent. Each other task is a class of its own:
  * `WebViewSetup`: the WebView, the spinner, and the address of each link.
  * `Fullscreen`: the system bars.
  * `ShareIn`: a note, an image or a JSON file from another application.
  * `ShareOut`: a note to another application.
  * `IntentBridge`: an `intent:` link, a capture result and Termux.
  * `Shortcuts`: a note on the home screen.
* Each class is a plain class. It extends no class of the framework, and
  the manifest names none of them. `MainActivity`, `ServerService` and
  `ExportProvider` stay the three components.
* A class with state is an object that `MainActivity` holds, and it keeps a
  reference to the activity. `Fullscreen` and `WebViewSetup` have no state,
  thus each one has one static method.
* `OmnText` holds the text rules. It imports no Android package, thus
  plain `javac` and `java` run `OmnTextTest`. `OmnConfig` is the same
  pattern. `importErrorMessage` reads JSON with `OmnConfig.parseFlat`,
  because a plain JVM cannot load `org.json`.
* `exportDescription` stays in `ShareOut`. It uses `android.util.Base64`.
  `java.util.Base64` needs API 26, and `minSdk` is 23.
* Each call of `OmnConfig` stays in `MainActivity`, in three short
  methods. `TestAndroidConfigCallSitesTypeCheck` reads the calls from one
  file.
* The text of each method is the same as before. A method keeps its name,
  thus each comment that names a method stays correct.
* A member of a new class is not private. Most of them are read by an
  inner class, and the compiler adds a bridge method for each private
  member that an inner class reads.
* No AndroidX, and no new dependency.

## Rejected alternatives

* **One file with clear sections.** The file had sections, and it still
  grew to 2243 lines.
* **Static methods only, with the activity as a parameter.** Two tasks
  hold state: the name of the capture result and two receivers. A static
  class would keep that state in `MainActivity` again.
* **Compile `MainActivity` in the gate.** It needs `android.jar`, `R` and
  `BuildConfig`. See the banner of `TestAndroidConfigCallSitesTypeCheck`.

## Consequences

* The APK holds seven more classes. `proguard-rules.pro` keeps each class
  of the package, thus R8 does not merge them. The DEX file grows by a
  small number of kilobytes. Measure the APK before and after.
* `TestMainActivityIsDivided` holds the limit of 800 lines for
  `MainActivity.java`, and it holds the list of the classes.
* `TestJavaUnitTests` runs `OmnConfigTest` and `OmnTextTest`.
* The gate still does not compile the classes that use the framework. Only
  the Gradle build finds a compile fault in them.
* `importErrorMessage` shows the status code for a `message` that is not a
  text. `org.json` changed such a value to text. The server sends a text.
