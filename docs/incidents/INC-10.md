# INC-10: the ingestion that started over

- **Date:** 2026-11-26
- **Module:** 10, Durability
- **Reproduction:** `garimpo ingest` over `encoding/json` (48 documents, 1,038 chunks) at
  tag `aula-09-09`, 4 workers, `all-minilm:22m` with Ollama on 4 CPUs, stopped after 20 s
  with `timeout -s TERM 20s` and run again; a temporary test ran the same two runs through
  `RunIngestion` and counted the texts sent to the model

## Summary

An ingestion takes minutes: the whole corpus took 31 minutes in module 08. A stop in the
middle, a `docker stop`, a deploy or a crash, threw all of it away. The run recorded an
event for every chunk (module 07), but the events lived in memory and went nowhere. The next
run started from the first document and sent the model every chunk again.

Nothing failed in a way anyone would see: the process ended with the signal and printed
nothing, and the next run completed. The cost was the time and the calls of the first run,
paid twice.

## Impact

The binary, killed by `SIGTERM` after 20 s and run again:

```
$ timeout -s TERM 20s garimpo ingest -corpus /usr/local/go/src/encoding/json
exit status 124
$ garimpo ingest -corpus /usr/local/go/src/encoding/json
completed documents=48 chunks=1038 embedded=1038 failed=0 pending=0 in 31.207s
```

The same two runs in a test, with the first one stopped by its context after 20 s:

```
run 1: failed documents=41 chunks=916 embedded=660 failed=0 pending=256 in 20.001s (err: context deadline exceeded)
run 2: completed documents=48 chunks=1038 embedded=1038 failed=0 pending=0 in 32.913s (err: <nil>)
distinct texts embedded: 1037; embedded twice: 659
```

The second run redid the 660 chunks of the first: 20 s of the model's time. Two chunks of
the corpus have the same text, so 1,038 chunks are 1,037 distinct texts, and 660 repeated
calls are 659 texts. Over the whole corpus, a stop at minute 30 would lose 30 minutes.

## Cause

1. **The events had nowhere to go.** `RunIngestion` handed each event to an optional
   callback, and `garimpo ingest` set none: the progress existed only in the process.
2. **A stop was a failure.** A canceled context failed the run (`RunFailed`), and a failed
   run is final. A `SIGTERM` did not even get that far: Go's default for it ends the
   process at once.

## Fix

1. **A journal (RN02).** The `ingestion.Journal` port takes the events and gives them back
   in order, across processes; `journaltest.Contract` checks it against a journal in memory
   and the file. `journal.File` writes one line per event, the CRC-32C of its JSON and the
   JSON, with one write and one `Sync` per append. `Open` drops a last line that a crash
   cut or damaged, whose events were never confirmed, and refuses a damaged line followed
   by good ones (`ErrCorrupt`).
2. **Resume by replaying it.** `ingestion.RestoreRun` rebuilds the run by running again the
   command each event records, so the state machine checks the journal. `OpenRun` pauses a
   run that a crash left running, and `RunIngestion` resumes a paused run, skipping the
   chunks it handled. `garimpo ingest -journal FILE`:

   ```
   $ timeout -s TERM 20s garimpo ingest -corpus /usr/local/go/src/encoding/json -journal /tmp/json.journal
   exit status 124
   $ grep -c ChunkEmbedded /tmp/json.journal
   631
   $ garimpo ingest -corpus /usr/local/go/src/encoding/json -journal /tmp/json.journal
   completed documents=48 chunks=407 embedded=407 failed=0 pending=0 skipped=631 in 13.779s
   $ garimpo ingest -corpus /usr/local/go/src/encoding/json -journal /tmp/json.journal
   the run in /tmp/json.journal is completed already: nothing to do
   ```

3. **A stop pauses.** `garimpo ingest` takes `SIGINT` and `SIGTERM` with
   `signal.NotifyContext`. `RunIngestion` treats a canceled context as a request to stop: no
   new document, chunk or call starts, the calls in flight finish on a context the stop does
   not cancel and are recorded, and the run pauses. A second signal ends the process. A
   `SIGTERM` after 10 s:

   ```
   paused documents=29 chunks=574 embedded=322 failed=0 pending=252 skipped=0 in 10.096s
   exit status 0, 98 ms after the SIGTERM
   ```

   The 252 pending chunks were cut and never sent; the next run cut them again and finished
   the 716 the journal lacked, in 22.197 s.

The journal costs one `Sync` per event, about a millisecond on this disk (`labs/fsync`).
Three runs each over `encoding/json`: 32.406, 32.046 and 31.574 s without it, 34.793,
31.949 and 32.328 s with it. With an embedder that answers at once, the journal is the
limit: 1,787 to 1,932 chunks/s, against 233,034 to 312,456 without it. The model embeds 33.

## Regression tests

`TestIngestResumesAfterKill9` runs `garimpo ingest` in a child process over 400 files and a
slow fake Ollama, kills it with `SIGKILL` once the journal confirms 100 chunks, and resumes
in the test:

```
=== RUN   TestIngestResumesAfterKill9
    crash_test.go:105: killed with 100 chunks confirmed in the journal
    crash_test.go:114: resumed: completed documents=400 chunks=300 embedded=300 failed=0 pending=0 skipped=100 in 452ms
--- PASS: TestIngestResumesAfterKill9 (0.73s)
```

The resumed run calls the model once per chunk the journal lacked, and the journal confirms
each chunk once. Without the skip, the resumed run fails at the first chunk of the journal:
`chunk already handled`. On the real Ollama, a `kill -9` after 10 s left 307 chunks in the
journal; the next run embedded the other 731, and no chunk appears twice among the 1,038.

`TestIngestPausesOnSIGTERM` sends the test process a `SIGTERM` during a run: before the
fix, it ended the test binary (`signal: terminated`). `TestAStopLetsTheCallsInFlightFinish`
holds three calls in flight, stops the run, and checks that none saw its context canceled
and all three were confirmed.

## Where Garimpo meets it

Every long job has to say what it has done in a place that outlives it, and before it moves
on: the journal is that place, and the `Sync` is the "before". The tests kill the process,
not the machine: after a `kill -9`, the kernel still has every write, synced or not; what a
power cut keeps depends on the `Sync` reaching the disk, which the course trusts and does
not test. The journal holds one line per chunk and is replayed whole on every resume. The
journal of the whole corpus, 41,980 chunks written with an embedder that answers at once,
has 5,340,035 bytes, and `Open` and `OpenRun` take 78 to 82 ms on it: a checkpoint that
compacts it would save less than a tenth of a second per resume, and waits until a replay
costs more.
