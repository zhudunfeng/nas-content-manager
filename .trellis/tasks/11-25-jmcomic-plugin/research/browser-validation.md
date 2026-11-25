# JMComic browser validation

Date: 2026-11-25. Runner: `scripts/verify_jmcomic_browser.py`, Python Playwright
with installed Microsoft Edge, launched through `scripts/project_env.py`.
The app and collection worker are real local binaries. Upstream search/download
is the explicit protocol fixture from `scripts/verify_jmcomic.py`; this browser
run does not execute the Python bridge or access a real comic site.

## Executed results

Final run: **38 assertions passed**, no unhandled page exceptions.

- Tested 375/768/1024/1440 px with light/dark themes: no document, modal or form
  horizontal overflow, existing 18/26 px form padding, native modal and focus
  restoration on Escape. Screenshots were visually inspected for desktop dark,
  phone light and the real fixture comic reader.
- Created a JMComic source through the real form after filling RSS private-network
  permission and an external entry URL. The submitted JMComic body removed
  unrelated values and successful save opened exactly one search modal.
- Real app/worker protocol: search pagination, last-page disabled navigation,
  successful empty search, real fixture 502 distinct from empty, selected and
  manual-ID previews, invalidation on ID change, fixed comic library target,
  submission followed by task details, completed import and actual image loading
  and second-page navigation in the original reader.
- Real slow fixture search followed by empty search: the late first result did
  not overwrite current state.
- Explicit **UI-only intercepted responses**: page 1000 with `hasNext=true`, long
  title/tag on mobile, expired preview, and aborted task submissions. Confirmed
  page bounds, wrapping, expired-submit prevention and identical full request
  bodies/UUID across uncertain retries. These are frontend assertions, not
  backend acceptance or proof that an interrupted request was received.

## Evidence and commands

Final evidence (Git ignored):
`.runtime/verify/jmcomic-browser-20261125-2/browser-091618/result.json`, eight theme
and width screenshots, and `jmcomic-reader.png` in the same directory.

```powershell
python -B scripts/project_env.py -- python -B scripts/verify_jmcomic.py --binary .runtime/bin/jmcomic-final/nas.exe --worker-binary .runtime/bin/jmcomic-final/nas-collection-worker.exe --root .runtime/verify/jmcomic-browser-NEW --serve
python -B scripts/project_env.py -- python -B scripts/verify_jmcomic_browser.py --info .runtime/verify/jmcomic-browser-NEW/browser.json
# Close only this owned verification environment after browser completion.
New-Item -ItemType File -Path .runtime/verify/jmcomic-browser-NEW/stop
```

Each full run requires a new serve environment because it creates sources and
downloads the fixed album once. Credentials remain in the isolated root, not in
this report. Both browser serve instances were stopped via their owned stop file;
the browser runner closes its browser in `finally`.

Earlier evidence is retained: the first browser run passed layout/search checks
but caught the original image-to-comic import filtering error; the next stopped
on a harness assertion that incorrectly assumed empty optional JSON fields were
serialized. Production filtering was fixed and the harness now accepts omitted
empty fields. Neither earlier run is counted as passing.

## Boundaries

No live-site search/download, real account, Docker runtime, Linux runtime or
physical NAS was exercised. Tiny generated raster pages establish image routing
and reader behavior, not scan/download performance or comic-content fidelity.
