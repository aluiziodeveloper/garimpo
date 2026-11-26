# INC-08: the text cut in silence

- **Date:** 2026-11-25
- **Module:** 08, Embeddings and the anticorruption layer
- **Reproduction:** `garimpo ingest` at tag `aula-08-03` over `time/tzdata`; a count of the
  tokens of every chunk of the corpus with `all-minilm:22m` (`num_ctx` 512, one chunk per
  request); a temporary test that embeds a chunk and the same chunk with another last line

## Summary

The first Ollama adapter sent each chunk to `/api/embed` with Ollama's defaults. By
default, Ollama truncates an input longer than the model's context, and the context of
`all-minilm:22m` is 256 tokens unless the request asks for more. Nothing failed: every chunk
got a vector, and every run completed. But 36% of the chunks of the corpus are longer than
256 tokens, and their vectors stood for their first 256 tokens only.

The window of 700 bytes (D15) was chosen to fit the model: the prototype measured an
average of 217 tokens per chunk. It measured with the default context, where Ollama stops
counting at 256, so the average hid every chunk over the limit.

## Impact

Tokens of every chunk of the corpus (2,972 documents, 41,980 chunks of at most 700 bytes),
`all-minilm:22m` on Ollama 0.40.16, `num_ctx` 512, so a count stops at 512:

| Measure | Value |
|---|---|
| Average tokens per chunk | 250.7 (220.4 when the count stops at 256) |
| Chunks over 256 tokens, cut in silence by default | 15,159 (36.11%), in 1,598 files |
| Tokens dropped at 256, at least | 1,271,555 of 10,525,861 (12.1%) |
| Chunks of 512 tokens or more, over the model's own context | 2,740 (6.53%) |

The chunks over the limit are mostly generated tables and data (`syscall`, `crypto`,
`time/tzdata`, `simd`), but not only: lines 400 to 416 of
`net/http/internal/http2/ciphers.go` have 347 tokens.

What a cut looks like: that chunk (699 bytes) and the same chunk with another last line
(700 bytes) got the same vector, `dot(a, b) = 1.000000`. The model never read the last line.
`garimpo ingest` over `time/tzdata` reported `completed documents=2 chunks=1971
embedded=1971 failed=0`.

## Cause

1. **A default that hides an error.** Ollama's `truncate` defaults to true: an input over
   the context is cut, and the answer is `200 OK`. The adapter did not say otherwise, and
   the port had no word for "this text does not fit".
2. **A measurement capped by the thing it measured.** The prototype counted tokens with the
   default context: `prompt_eval_count` never passed 256, and the average looked safe.

## Fix

1. **No cut in silence.** `ingestion.ErrTextTooLong` is part of the `Embedder` port, and the
   contract has a case for it: a text over the context fails, never comes back cut. The
   Ollama adapter always sends `"truncate": false` and turns Ollama's
   `400 Bad Request: the input length exceeds the context length` into that error. The run
   records the chunk as failed, with the reason.
2. **The whole context of the model.** The adapter sends `num_ctx` 512, the context of
   `all-minilm` (`bert.context_length` in `/api/show`): chunks of 257 to 511 tokens are
   embedded whole instead of cut. The same chunk of `ciphers.go` and its copy with another
   last line now differ: `dot(a, b) = 0.947098`.

The full corpus with the fix, 4 workers: `completed documents=2972 chunks=41980
embedded=39244 failed=2736 pending=0 in 31m14.385s`. The 2,736 failed chunks do not fit
in 512 tokens; they are counted and recorded instead of hidden. The count found 2,740 at 512
or more, but it stops at 512, so a chunk of exactly 512 fits and counts there too.

## Regression tests

`embeddertest.Contract`, case `ATextOverTheContextFails`, runs against the fake, the
Ollama adapter over a fake server that truncates like Ollama, the `Batcher`, and a real
Ollama when `GARIMPO_OLLAMA_URL` is set. Before the fix:

```
--- FAIL: TestEmbedderMeetsTheContract (0.01s)
    --- FAIL: TestEmbedderMeetsTheContract/ATextOverTheContextFails (0.00s)
        contract.go:54: Embed of a text over the context: err = <nil>, want ErrTextTooLong
--- FAIL: TestOllamaMeetsTheContract (0.25s)
    --- FAIL: TestOllamaMeetsTheContract/ATextOverTheContextFails (0.07s)
        contract.go:54: Embed of a text over the context: err = <nil>, want ErrTextTooLong
```

`TestEmbedSpeaksOllamasAPI` checks the request itself: `"truncate":false` and
`"num_ctx":512`.

## Where Garimpo meets it

Every adapter to a model states its limits and fails past them: a silent default is a
decision someone else made. The 2,736 chunks over 512 tokens are not in the index; a cutter
that knows tokens, or one that splits a failed chunk and tries again, would bring them
back. The `Batcher` keeps the rule: when one text of a batch is too long, it sends each
text again on its own, so only that text fails.
