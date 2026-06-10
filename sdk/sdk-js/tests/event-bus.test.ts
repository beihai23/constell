import { describe, it, expect, vi } from "vitest";
import { EventBus } from "../src/event-bus.js";

// Define a typed event map for testing
interface TestEvents {
  message: (text: string) => void;
  count: (n: number) => void;
  empty: () => void;
}

describe("EventBus", () => {
  // ---------------------------------------------------------------------------
  // on / emit — basic dispatch
  // ---------------------------------------------------------------------------
  describe("on + emit", () => {
    it("calls registered handler on emit", () => {
      const bus = new EventBus<TestEvents>();
      const handler = vi.fn();

      bus.on("message", handler);
      bus.emit("message", "hello");

      expect(handler).toHaveBeenCalledOnce();
      expect(handler).toHaveBeenCalledWith("hello");
    });

    it("does not call handlers registered for a different event", () => {
      const bus = new EventBus<TestEvents>();
      const messageHandler = vi.fn();
      const countHandler = vi.fn();

      bus.on("message", messageHandler);
      bus.on("count", countHandler);
      bus.emit("message", "hi");

      expect(messageHandler).toHaveBeenCalledOnce();
      expect(countHandler).not.toHaveBeenCalled();
    });
  });

  // ---------------------------------------------------------------------------
  // Multiple handlers for the same event
  // ---------------------------------------------------------------------------
  describe("multiple handlers", () => {
    it("supports multiple handlers for the same event", () => {
      const bus = new EventBus<TestEvents>();
      const h1 = vi.fn();
      const h2 = vi.fn();

      bus.on("message", h1);
      bus.on("message", h2);
      bus.emit("message", "hello");

      expect(h1).toHaveBeenCalledOnce();
      expect(h2).toHaveBeenCalledOnce();
      expect(h1).toHaveBeenCalledWith("hello");
      expect(h2).toHaveBeenCalledWith("hello");
    });

    it("does not duplicate a handler registered twice", () => {
      const bus = new EventBus<TestEvents>();
      const handler = vi.fn();

      bus.on("message", handler);
      bus.on("message", handler); // same reference again
      bus.emit("message", "dup");

      expect(handler).toHaveBeenCalledOnce();
    });
  });

  // ---------------------------------------------------------------------------
  // off
  // ---------------------------------------------------------------------------
  describe("off", () => {
    it("removes a specific handler", () => {
      const bus = new EventBus<TestEvents>();
      const h1 = vi.fn();
      const h2 = vi.fn();

      bus.on("message", h1);
      bus.on("message", h2);
      bus.off("message", h1);
      bus.emit("message", "after");

      expect(h1).not.toHaveBeenCalled();
      expect(h2).toHaveBeenCalledOnce();
    });

    it("is a no-op when the handler was never registered", () => {
      const bus = new EventBus<TestEvents>();
      const handler = vi.fn();

      // Removing a handler that was never added should not throw
      expect(() => bus.off("message", handler)).not.toThrow();
    });
  });

  // ---------------------------------------------------------------------------
  // removeAllListeners
  // ---------------------------------------------------------------------------
  describe("removeAllListeners", () => {
    it("clears all handlers", () => {
      const bus = new EventBus<TestEvents>();
      const h1 = vi.fn();
      const h2 = vi.fn();

      bus.on("message", h1);
      bus.on("count", h2);
      bus.removeAllListeners();

      bus.emit("message", "gone");
      bus.emit("count", 42);

      expect(h1).not.toHaveBeenCalled();
      expect(h2).not.toHaveBeenCalled();
    });
  });

  // ---------------------------------------------------------------------------
  // emit with no handlers — must not throw
  // ---------------------------------------------------------------------------
  describe("emit with no handlers", () => {
    it("does not throw when emitting an event with no handlers", () => {
      const bus = new EventBus<TestEvents>();

      expect(() => bus.emit("message", "nobody")).not.toThrow();
      expect(() => bus.emit("empty")).not.toThrow();
    });
  });

  // ---------------------------------------------------------------------------
  // handler error isolation
  // ---------------------------------------------------------------------------
  describe("error isolation", () => {
    it("swallows handler errors so other handlers still execute", () => {
      const bus = new EventBus<TestEvents>();
      const goodHandler = vi.fn();
      const badHandler = vi.fn(() => {
        throw new Error("boom");
      });

      bus.on("message", badHandler);
      bus.on("message", goodHandler);
      bus.emit("message", "test");

      expect(badHandler).toHaveBeenCalledOnce();
      expect(goodHandler).toHaveBeenCalledOnce();
    });
  });
});
