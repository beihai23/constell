# Instructions

- Following Playwright test failed.
- Explain why, be concise, respect Playwright best practices.
- Provide a snippet of code with the fix, if possible.

# Test info

- Name: realtime.spec.ts >> Real-time messaging >> user B sees channel message from user A in real-time
- Location: e2e/realtime.spec.ts:4:3

# Error details

```
TimeoutError: page.waitForSelector: Timeout 15000ms exceeded.
Call log:
  - waiting for locator('text=Realtime 1781013998954') to be visible

```

# Page snapshot

```yaml
- generic [ref=e4]:
  - generic [ref=e5]:
    - button "Direct Messages" [ref=e6]:
      - generic [ref=e8]: 💬
    - separator [ref=e9]
    - button "Add a Community" [ref=e10]:
      - generic [ref=e12]: +
    - button [ref=e13]
  - generic [ref=e17]:
    - heading "Select a Community" [level=2] [ref=e19]
    - generic [ref=e22]: Search...
    - generic [ref=e24]:
      - generic [ref=e26]: Text Channels
      - paragraph [ref=e27]: No channels yet
  - generic [ref=e30]:
    - generic [ref=e31]:
      - generic [ref=e32]:
        - img [ref=e33]
        - generic [ref=e36]: Select a channel
      - generic [ref=e37]:
        - button "Search" [ref=e38]:
          - img [ref=e39]
        - button "Toggle member list" [ref=e42]:
          - img [ref=e43]
    - paragraph [ref=e49]: No messages yet
    - generic [ref=e51]:
      - button "Attach file" [ref=e52]:
        - img [ref=e53]
      - 'textbox "Message #channel" [active] [ref=e54]': Realtime 1781013998954
      - button "Send message" [ref=e55]:
        - img [ref=e56]
```

# Test source

```ts
  1   | import { test, expect, registerTestUser, createCommunity, createChannel, joinCommunity } from './fixtures';
  2   | 
  3   | test.describe('Real-time messaging', () => {
  4   |   test('user B sees channel message from user A in real-time', async ({ browser, user }) => {
  5   |     // Create a second user.
  6   |     const userB = await registerTestUser('b');
  7   | 
  8   |     // Setup community + channel.
  9   |     const community = await createCommunity(user.accessToken, 'Realtime Test');
  10  |     const channel = await createChannel(user.accessToken, community.id, 'rt-channel');
  11  | 
  12  |     // User B joins the community.
  13  |     await joinCommunity(userB.accessToken, community.id, userB.userId);
  14  | 
  15  |     // Open two browser contexts.
  16  |     const ctxA = await browser.newContext();
  17  |     const ctxB = await browser.newContext();
  18  | 
  19  |     const pageA = await ctxA.newPage();
  20  |     const pageB = await ctxB.newPage();
  21  | 
  22  |     // Inject auth for both users and navigate to the channel.
  23  |     // User A
  24  |     await pageA.goto('/login');
  25  |     await pageA.evaluate(
  26  |       (tokens: { at: string; rt: string }) => {
  27  |         localStorage.setItem('constell_access_token', tokens.at);
  28  |         localStorage.setItem('constell_refresh_token', tokens.rt);
  29  |       },
  30  |       { at: user.accessToken, rt: user.refreshToken },
  31  |     );
  32  |     await pageA.goto(`/${community.id}/${channel.id}`);
  33  | 
  34  |     // User B
  35  |     await pageB.goto('/login');
  36  |     await pageB.evaluate(
  37  |       (tokens: { at: string; rt: string }) => {
  38  |         localStorage.setItem('constell_access_token', tokens.at);
  39  |         localStorage.setItem('constell_refresh_token', tokens.rt);
  40  |       },
  41  |       { at: userB.accessToken, rt: userB.refreshToken },
  42  |     );
  43  |     await pageB.goto(`/${community.id}/${channel.id}`);
  44  | 
  45  |     // Wait for both pages to load the chat view.
  46  |     await pageA.waitForSelector('[aria-label="Send message"]', { timeout: 10000 }).catch(() => {});
  47  |     await pageB.waitForSelector('[aria-label="Send message"]', { timeout: 10000 }).catch(() => {});
  48  | 
  49  |     // User A sends a message via the UI.
  50  |     const rtMessage = `Realtime ${Date.now()}`;
  51  |     const chatInput = pageA.locator('textarea').first();
  52  |     await chatInput.fill(rtMessage);
  53  |     await pageA.keyboard.press('Enter');
  54  | 
  55  |     // User B should see the message appear in real-time.
> 56  |     await pageB.waitForSelector(`text=${rtMessage}`, { timeout: 15000 });
      |                 ^ TimeoutError: page.waitForSelector: Timeout 15000ms exceeded.
  57  |     await expect(pageB.locator(`text=${rtMessage}`).first()).toBeVisible();
  58  | 
  59  |     // Cleanup.
  60  |     await ctxA.close();
  61  |     await ctxB.close();
  62  |   });
  63  | 
  64  |   test('user A receives DM from user B in real-time', async ({ browser, user }) => {
  65  |     const userB = await registerTestUser('dm');
  66  | 
  67  |     // Open page for user A (listening for DM).
  68  |     const ctxA = await browser.newContext();
  69  |     const pageA = await ctxA.newPage();
  70  | 
  71  |     await pageA.goto('/login');
  72  |     await pageA.evaluate(
  73  |       (tokens: { at: string; rt: string }) => {
  74  |         localStorage.setItem('constell_access_token', tokens.at);
  75  |         localStorage.setItem('constell_refresh_token', tokens.rt);
  76  |       },
  77  |       { at: user.accessToken, rt: user.refreshToken },
  78  |     );
  79  |     await pageA.goto(`/@me`);
  80  | 
  81  |     // Wait for app to load and WS to connect.
  82  |     await pageA.waitForTimeout(3000);
  83  | 
  84  |     // User B sends DM via API.
  85  |     const dmContent = `RT DM ${Date.now()}`;
  86  |     await fetch(`${process.env.E2E_API_URL || 'http://localhost:3000'}/api/v1/dm/send`, {
  87  |       method: 'POST',
  88  |       headers: {
  89  |         'Content-Type': 'application/json',
  90  |         Authorization: `Bearer ${userB.accessToken}`,
  91  |       },
  92  |       body: JSON.stringify({ target_user_id: user.userId, content: dmContent }),
  93  |     });
  94  | 
  95  |     // User A's page should show the new DM (either as notification badge or in DM list).
  96  |     // Navigate to the DM conversation to verify.
  97  |     await pageA.goto(`/@me/${userB.userId}`);
  98  |     await pageA.waitForSelector(`text=${dmContent}`, { timeout: 15000 });
  99  |     await expect(pageA.locator(`text=${dmContent}`).first()).toBeVisible();
  100 | 
  101 |     await ctxA.close();
  102 |   });
  103 | });
  104 | 
```