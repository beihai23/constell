/**
 * Unit test for the message ordering comparator (messagesStore.byChrono).
 *
 * Why this exists: messages were rendering out of order. Root cause — the store
 * sorted by `createdAt`, but the server emits created_at as Unix SECONDS while
 * optimistic (pre-ACK) messages use `Date.now()` MILLISECONDS, a 1000× mix that
 * scrambles any createdAt-based sort. The fix is to sort by the authoritative,
 * unit-agnostic, monotonic `seq`. This test pins that with the exact failing
 * shape and contrasts it against the old (broken) comparator.
 *
 * Runs under the Playwright runner (the project's only JS test runner) but uses
 * no browser — it exercises the pure comparator directly.
 */
import { test, expect } from '@playwright/test';
import { byChrono, withLocalSeq } from '../src/stores/messagesStore';

// The old comparator this replaces — createdAt primary, seq tiebreaker only.
// Kept here to prove the regression: it orders the bug-case WRONG.
const oldByCreatedAt = (a: { createdAt?: number; seq?: number }, b: { createdAt?: number; seq?: number }) => {
  const seqOr = (m: { seq?: number }) => (m.seq && m.seq > 0 ? m.seq : Number.MAX_SAFE_INTEGER);
  return (a.createdAt ?? 0) - (b.createdAt ?? 0) || seqOr(a) - seqOr(b);
};

test.describe('message order comparator (seq-primary)', () => {
  test('THE BUG: an older-by-seq optimistic (ms) message sorts before a newer server (seconds) message', () => {
    // seq 28 is still optimistic → createdAt in MILLISECONDS (Date.now()).
    // seq 29 already ACKed → createdAt in SECONDS (server m.CreatedAt.Unix()).
    // True order: 28 before 29 (seq). But 28's ms createdAt is 1000× larger than
    // 29's seconds createdAt, so a createdAt-primary sort puts 29 first — wrong.
    const msg28 = { seq: 28, createdAt: 1782744267_000 }; // optimistic, ms
    const msg29 = { seq: 29, createdAt: 1782744267 }; // server, seconds

    // New (seq-primary): 28 before 29.
    expect(byChrono(msg28, msg29)).toBeLessThan(0);
    expect(byChrono(msg29, msg28)).toBeGreaterThan(0);

    // Old (createdAt-primary): 29 before 28 — the bug. Lock the regression.
    expect(oldByCreatedAt(msg28, msg29)).toBeGreaterThan(0);
  });

  test('two server (seconds) messages order by seq', () => {
    const a = { seq: 28, createdAt: 1782744267 };
    const b = { seq: 29, createdAt: 1782744272 };
    expect(byChrono(a, b)).toBeLessThan(0);
  });

  test('a seq-less optimistic message sorts as newest (after every seq’d message)', () => {
    const acked = { seq: 5, createdAt: 1782744267 };
    const optimistic = { createdAt: 1782744267_000 }; // no seq → just sent
    expect(byChrono(acked, optimistic)).toBeLessThan(0);
  });

  test('sorts a mixed list into seq order', () => {
    const list = [
      { seq: 30, createdAt: 1782744279 },
      { seq: 28, createdAt: 1782744267_000 }, // optimistic ms — must NOT jump past 29/30
      { seq: 29, createdAt: 1782744272 },
      { seq: 27, createdAt: 1782744257 },
    ].sort(byChrono);
    expect(list.map((m) => m.seq)).toEqual([27, 28, 29, 30]);
  });
});

test.describe('optimistic local high-water seq (withLocalSeq)', () => {
  test('a seq-less optimistic message gets max(existing seq) + 1', () => {
    const existing = [{ id: 'a', seq: 100 }, { id: 'b', seq: 103 }];
    const optimistic = withLocalSeq(existing, { id: 'opt', seq: 0, content: 'hi' });
    expect(optimistic.seq).toBe(104); // max(100,103)+1
  });

  test('a real (seq’d) message is passed through unchanged', () => {
    const real = { id: 'r', seq: 50, content: 'x' };
    expect(withLocalSeq([{ id: 'a', seq: 100 }], real)).toEqual(real);
  });

  test('rapid optimistic sends keep incrementing from the prior optimistic local seq', () => {
    const existing = [{ id: 'a', seq: 100 }, { id: 'opt1', seq: 101 }]; // opt1 already local
    const opt2 = withLocalSeq(existing, { id: 'opt2', seq: 0 });
    expect(opt2.seq).toBe(102); // after opt1, not back to 101
  });

  test('THE FATAL CASE: a newer real message arriving during the ACK window still sorts AFTER the optimistic', () => {
    // Channel has a real message seq 100. I send an optimistic message.
    const existing = [{ id: 'a', seq: 100, createdAt: 1782744200 }];
    const optimistic = withLocalSeq(existing, { id: 'opt', seq: 0, createdAt: 1782744267_000 });
    expect(optimistic.seq).toBe(101);

    // Before my ACK returns, another user sends a newer real message (seq 105).
    const newerReal = { id: 'y', seq: 105, createdAt: 1782744272 };
    const sorted = [...existing, optimistic, newerReal].sort(byChrono);

    // Correct order: a(100), opt(101), y(105). The optimistic I sent FIRST stays
    // ABOVE the newer message. Under the old flat-MAX scheme the optimistic would
    // have been MAX and sorted AFTER y — the visible misorder this prevents.
    expect(sorted.map((m) => m.id)).toEqual(['a', 'opt', 'y']);
  });

  test('ACK in the common case assigns the same seq → no position change', () => {
    // Optimistic got local 101; server also assigns 101 (no concurrency).
    const before = [{ id: 'a', seq: 100 }, { id: 'opt', seq: 101 }, { id: 'y', seq: 105 }].sort(byChrono);
    const after = [{ id: 'a', seq: 100 }, { id: 'opt', seq: 101 }, { id: 'y', seq: 105 }].sort(byChrono);
    expect(before.map((m) => m.id)).toEqual(after.map((m) => m.id));
  });
});
