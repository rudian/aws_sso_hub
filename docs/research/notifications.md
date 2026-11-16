# System notifications: research

Date: 2026-11-16. Author: UI designer and product researcher (Fable).
Question: can Codepass send a macOS notification when an AWS sign-in expires, with a button that starts the sign-in again? And the same on Windows?

Short answer: **yes on both.** The Wails 3.1.0 notifications service supports buttons and tells the app which button was pressed. There are limits. They are listed in section 1.

How to read the sources:

- `W/` means `~/go/pkg/mod/github.com/wailsapp/wails/v3@v3.1.0/pkg/services/notifications/`.
- `A/` means `~/go/pkg/mod/github.com/wailsapp/wails/v3@v3.1.0/pkg/application/`.
- "Apple" and "Microsoft" lines name the page. The URLs are in section 9.
- **Nothing was run.** Every fact comes from reading source or documentation. What reading could not settle is a manual check in section 8.

---

## 1. What limits the button

| # | Limit | Platform | Source | What the plan does |
|---|---|---|---|---|
| 1 | The button text belongs to the category, not to one notification. It is fixed when the category is registered. | both | `W/notifications.go:116-122`, `W/notifications_darwin.m:443-457`, `W/notifications_windows.go:532-541` | Register the categories at start and again when the language changes. |
| 2 | A category that is not registered gives a notification **without** buttons. No error. | both | `W/notifications_darwin_shared.go:38-40`, `W/notifications_windows.go:274-277` | Register before the first send. A test checks it. |
| 3 | With the default "Banners" style, macOS shows the button only when the pointer is over the banner. With two or more buttons it shows an "Options" menu. | macOS | Not in the Apple pages read. From use of macOS. **Manual check M2.** | One button per notification. A click on the notification itself also does something useful. |
| 4 | Wails creates buttons without the "foreground" option. Pressing the button does not bring Codepass to the front. | macOS | `W/notifications_darwin.m:449-455`; Apple, "Handling notifications and notification-related actions" | Good for us: the sign-in opens in a browser window. The app shows its own window only for a click on the notification body. |
| 5 | The app can only ask "allowed: yes or no". It cannot tell "never asked" from "turned off". | macOS | `W/notifications_darwin.m:142-145` | Codepass remembers by itself that it asked. |
| 6 | The service refuses to start without a bundle identifier. A service that fails to start stops the whole app. | macOS | `W/notifications_darwin.go:45-51`, `A/application.go:698-700` | Codepass starts the service inside its own wrapper and catches the error. A bare binary runs without system notifications. |
| 7 | Windows cannot replace or take back a notification that was shown. | Windows | `W/notifications_windows.go:282-291`, `345-355` | On Windows an old notice may stay in the notification centre. A click on a stale one is handled (plan 4.15). |
| 8 | On Windows the app cannot tell whether notifications are turned off. The call always answers "allowed". | Windows | `W/notifications_windows.go:172-182` | No "notifications are off" state on Windows. One line of help text. |
| 9 | Only one response callback exists for the whole app. | both | `W/notifications.go:95-97`, `229-238` | One Codepass service owns it and routes by category. |
| 10 | The development build has the same bundle identifier as the installed app. | macOS | `build/darwin/Info.dev.plist` (`CFBundleIdentifier`), `internal/version/version.go:10` | Sandbox mode and tests use a fake. See section 6. |

Nothing makes the button impossible.

---

## 2. macOS

### 2.1 Buttons

- A notification gets buttons through a **category**. A category has an ID and a list of actions. Each action has an ID and a title. `W/notifications.go:108-122`.
- Wails builds a `UNNotificationAction` per action and a `UNNotificationCategory` with no options. `W/notifications_darwin.m:441-477`.
- Registering a category with an ID that exists replaces it. `W/notifications_darwin.m:480-499`. So re-registering with new titles (language change) is allowed by the code.
- How many: Apple says "When the system has unlimited space, the system displays up to 10 actions. When the system has limited space, the system displays at most two actions." Apple, `UNNotificationCategory`. The page does not say what macOS does.
- Title length: Apple states no limit. Keep titles short. "重新登录" and "Sign in again" are short.
- When to register: Apple says "at launch time". Apple, "Declaring your actionable notification types". Wails does not register anything by itself.
- A "destructive" flag exists (red text). We do not use it. `W/notifications.go:112`.
- A reply text field exists. We do not use it. `W/notifications_darwin.m:460-471`.

### 2.2 What the callback receives

- One function is registered with `OnNotificationResponse`. A second call replaces the first. `W/notifications.go:229-238`.
- It receives a `NotificationResult` with `Response` and `Error`. `W/notifications.go:217-222`.
- `Response` holds: `ID` (the notification ID the app chose), `ActionIdentifier`, `CategoryID`, `Title`, `Subtitle`, `Body`, `UserText`, `UserInfo`. `W/notifications.go:205-215`, filled at `W/notifications_darwin.m:54-76`.
- `UserInfo` is the `Data` map the app sent. `W/notifications_darwin.m:328-331` and `69-71`.
- A click on the notification itself arrives with `ActionIdentifier == "DEFAULT_ACTION"`. `W/notifications.go:203`, `W/notifications_darwin.go:239-241`.
- A button arrives with the action ID the app registered.
- Closing a notification sends nothing (the category has no dismiss option). `W/notifications_darwin.m:473-477`.

### 2.3 Window hidden, or app not running

- **Window hidden, app running:** the callback runs. For a button the app is not brought forward (limit 4). For a body click macOS activates the app, but nothing in Wails shows a hidden window. Codepass must show its window itself in the callback.
- **App not running:** Apple says "When the user selects an action, the system launches your app in the background and calls the delegate's" method. Apple, "Handling notifications and notification-related actions".
- Apple also says: "You must assign your delegate object to the UNUserNotificationCenter object before your app finishes launching." Apple, `UNUserNotificationCenterDelegate`.
- Wails sets the delegate in the service's startup. `W/notifications_darwin.go:52`, `W/notifications_darwin.m:98-112`. Services start before the native run loop starts. `A/application.go:693-704` runs before the platform `run`. So the delegate is in place in time, **if** the service is started at app start and not later.
- Codepass does work before `application.New` (the old-app check, `main.go`). A launch by notification goes through the same path. **Manual check M4.**
- A notification that is in Notification Center stays there after the app quits. So a click can come hours later.

### 2.4 Permission

- macOS asks the person the first time the app calls `requestAuthorization`. Later calls do not ask again. Apple, `requestAuthorization(options:completionHandler:)`.
- Wails asks for alert, sound and badge. `W/notifications_darwin.m:121-131`.
- `RequestNotificationAuthorization` waits up to 3 minutes for the answer. `W/notifications_darwin_shared.go:114-129`. Never call it on the main thread. Call it from a goroutine.
- `CheckNotificationAuthorization` answers `true` only for "authorized". `W/notifications_darwin.m:142-145`. "Not determined", "denied" and "provisional" all give `false`.
- Meaning of the states, from Apple, `UNAuthorizationStatus`:
  - not determined: "The user hasn't yet made a choice about whether the app is allowed to schedule notifications."
  - denied: "The app isn't authorized to schedule or receive notifications."
  - authorized: "The app is authorized to schedule or receive notifications."
- How Codepass tells them apart: it stores "asked: yes" after the first request. Not asked and `false` = not determined. Asked and `false` = turned off.
- Sending while not allowed: `addNotificationRequest` gives an error, and Wails returns it. `W/notifications_darwin.m:370-377`. The exact error text is not known. **Manual check M3.**
- The person can also turn off only banners, only sound, or use a Focus. The app cannot see that through Wails. So the in-app state (the pass, the badge) must never depend on the notification.
- Permission belongs to the bundle identifier. Codepass keeps the identifier of AWS SSO Hub (`internal/version/version.go:3-10`). AWS SSO Hub never asked for notifications, so an upgraded Mac starts at "not determined". **Manual check M9** confirms it.
- The URL that opens System Settings at Notifications is not in the Apple pages read. The commonly used one is `x-apple.systempreferences:com.apple.Notifications-Settings.extension`. **Manual check M8.**

### 2.5 Shown while the app is in front

- Wails tells macOS to show the banner, list entry and sound even when the app is the front app. `W/notifications_darwin.m:35-48`.
- So Codepass decides by itself when not to send (for example an AI task that waits while its window is focused).

### 2.6 Replace and remove

- Sending again with the same ID replaces the old notification. `W/notifications_darwin_shared.go:49-55`.
- `RemoveDeliveredNotification(id)` removes one from Notification Center. `W/notifications_darwin.go:169-175`.
- `RemoveNotification` does nothing on macOS. `W/notifications_darwin_shared.go:22-31`. Use the two specific calls.

### 2.7 Sound, grouping, priority

- Default: the system sound. `Sound{Silent: true}` gives none. `W/notifications_darwin.m:186-201`, `326`.
- `ThreadID` groups notifications in Notification Center. `W/notifications_darwin.m:333-336`. On Windows the same field becomes a **visible header** with the ID as its text. `W/notifications_windows.go:514-519`. So Codepass does not set it.
- `InterruptionLevel "timeSensitive"` can break through a Focus. `W/notifications_darwin.m:258-276`. We do not use it: an expired sign-in is not urgent enough to break a Focus.
- `Subtitle` is shown on macOS and dropped on Windows. `W/notifications.go:132`, `W/notifications_windows.go:122`. So Codepass puts everything in title and body.

### 2.8 Hardened runtime, notarization

- The Apple pages read say nothing about an entitlement, signing or the hardened runtime for local notifications.
- The release build is signed with the hardened runtime and **no entitlements** (`docs/research/spike-signing.md`, "Entitlements actually needed"). Local notifications use no entitlement that we know of. Push notifications do, and we send none.
- Wails says "Requires a properly bundled and signed application". `W/notifications.go:16`. It does not say which signature.
- **Manual check M1** (ad-hoc signed development bundle) and **M10** (notarized release build).

### 2.9 Development builds

- `task dev` runs `wails3 task run`. `build/config.yml:54-66`.
- On macOS that task builds `bin/codepass.dev.app` with `Info.dev.plist`, signs it ad hoc, and starts the binary inside the bundle. `build/darwin/Taskfile.yml:122-139`.
- So a development build **is** a bundle with an identifier. The service's bundle check passes. `W/notifications_darwin.m:19-25`.
- `go test`, `go run .` and a bare `bin/codepass` have no bundle. There the service fails to start (limit 6).
- The development bundle has the identifier `com.rudian.awsssohub`, the same as the installed app. Three results:
  - A permission answer given to the development build is also the answer for the installed app.
  - A notification from the development build can show the installed app's name and icon.
  - A click while the development build is not running can start the installed app.
- That is why development with real notifications is opt-in (section 6).

---

## 3. Windows

### 3.1 Buttons

- Up to five buttons. Microsoft: "Container element for declaring up to five inputs and up to five button actions for the toast notification." Microsoft, toast schema, `action` element.
- No text limit is stated for a button. Keep it short.
- Wails writes one `<action>` per category action, with `activationType="foreground"`. `W/notifications_windows.go:532-542`.
- The title gets one line (`hint-maxLines="1"`). The body has no limit set by Wails. `W/notifications_windows.go:483-493`. Windows cuts long text by itself. **Manual check W2.**
- Categories are kept in memory and saved to the registry under `HKCU\SOFTWARE\<app name>\NotificationCategories`. `W/notifications_windows.go:39-40`, `295-308`, `621-642`. They are loaded at start. `W/notifications_windows.go:152`.

### 3.2 What the callback receives

- The same `NotificationResponse` as on macOS. `W/notifications_windows.go:114-143`.
- Wails packs the action ID **and the whole notification** (ID, title, body, category, data) into the button's argument, as base64 JSON. `W/notifications_windows.go:569-583`, `249-254`.
- That argument is stored by Windows with the toast. So **`Data` must hold only identifiers**. No token, no URL, no code. This is also the rule on macOS.

### 3.3 App identity (AppUserModelID, shortcut, signing)

- Wails uses the app's name as the app ID. `W/notifications_windows.go:77`, `105-112`. For us that is "Codepass" (`internal/version/version.go:9`).
- At start Wails writes a GUID under `HKCU\Software\Classes\AppUserModelId\<app name>` (`CustomActivator`) and `HKCU\Software\Classes\CLSID\<GUID>\LocalServer32` = `"<exe path>" %1`. `W/notifications_windows.go:36-38`, `93-103`, `691-716`.
- The exe path is written at every start. So a moved or updated exe corrects itself the next time it runs.
- Wails then calls the library `go-toast` 2.0.3 (`SetAppData`). `W/notifications_windows.go:105-112`. Its source was not readable during this research (not in the local module cache, and the host did not answer). What it writes is not confirmed.
- Microsoft says for classic desktop apps: "Without a valid shortcut installed in the Start screen or in All Programs, you cannot raise a toast notification from a desktop app." Microsoft, "How to enable desktop toast notifications through an AppUserModelID".
- That page is old. Newer Windows versions accept the registry entries alone. This was **not** confirmed from a page. **Manual check W1** settles it for the portable exe and for the installed app.
- Code signing: no Microsoft page read ties toasts to a signature. The registration is per user and needs no administrator. An unsigned exe is expected to work. **Manual check W1** covers it.
- Wails pushes through the WinRT interface only. If that fails, the error goes back to the caller. `W/notifications_windows.go:259-263`.

### 3.4 App not running

- Wails registers its COM class at start, "so activation callbacks fire even if the user clicks a toast that pre-dates the first call". `W/notifications_windows.go:145-150`.
- When the app is not running, Windows starts the exe from `LocalServer32`. Codepass then starts as usual and registers the class. Whether the click still reaches the callback is **not** confirmed. **Manual check W3.**
- The old-app check and the single-instance check run first (`main.go`). **Manual check W3** includes them.

### 3.5 Permission and settings

- There is no permission prompt. Both authorization calls return `true`. `W/notifications_windows.go:172-182`.
- The person can turn notifications off per app in Windows Settings. Wails gives no way to read that.
- The Settings page opens with `ms-settings:notifications`. From memory, not from a page read. **Manual check W4.**

### 3.6 Replace and remove

- `UpdateNotification` shows a new toast. It does not replace. `W/notifications_windows.go:282-291`.
- `RemoveDeliveredNotification` does nothing. `W/notifications_windows.go:345-355`.
- A scheduled notification is a timer inside the app. It is lost when the app exits. `W/notifications_windows.go:190-231`. Codepass does not use scheduling. It sends at the moment its own ticker sees the event, on both platforms.

---

## 4. Why not let macOS schedule the "ended" notice

- macOS can deliver a scheduled notification even when the app is closed. `W/notifications_darwin.m:278-312`.
- We do not use it. The end time is often an estimate until AWS shows it (`internal/accounts/session.go` on `main`, `sessionEnd`). A scheduled notice would say "expired" when the sign-in is still good, or after the person already signed in again.
- The app's 30-second ticker already knows the real state. It sends then.

---

## 5. What the plan must respect

1. One Codepass service wraps the Wails service. It owns the single callback and routes by category ID.
2. It starts the Wails service itself and catches a failure. It is **not** put in `application.Options.Services` as the bare Wails service.
3. It is started at app start, so the delegate (macOS) and the COM class (Windows) exist before a click arrives.
4. Categories are registered at start and on a language change.
5. One button per notification on macOS.
6. Title and body only. No subtitle, no thread ID.
7. `Data` holds identifiers only (kind, portal name, task ID).
8. The permission request runs in a goroutine. The service stores that it asked.
9. On macOS, "ended" reuses the ID of "heads-up", so it replaces it. A successful sign-in removes the delivered notice.
10. The app state on screen never depends on a notification being seen.

---

## 6. Sandbox mode and tests: a fake

Why a fake is needed:

- Tests and `go run` have no bundle. The real service cannot start (limit 6).
- The development bundle shares its identifier with the installed app (limit 10). A real permission prompt or a real click would touch the installed app. That is forbidden in development.
- A test cannot press a button in a system banner.

The fake:

- Same interface as the real one: send, remove, permission state, request permission, open system settings.
- It records every notification (kind, ID, title, body, button, data) in memory.
- It has a "respond" call: a test, or the mock UI, presses the button or the body. The same routing code runs as for a real click.
- Its permission state can be set: allowed, not asked, off.
- In sandbox mode (`AWSSSOHUB_SANDBOX` set) the fake is **always** used. It also shows each notification as an in-app toast with the same text and button, so the flow can be seen and driven by the end-to-end tests.
- Real notifications in a development build are opt-in with one environment variable, used only for the manual checks below, and only when the installed app is not on that Mac or the person accepts the shared identity.

---

## 7. Not verified by this research

| Point | Why | Check |
|---|---|---|
| Ad-hoc signed bundle may send and receive | Not run | M1 |
| How the button looks (hover, "Options") | Not in Apple's pages | M2 |
| Error text when permission is off | Not run | M3 |
| Click while the app is closed starts the app and reaches the callback | Not run | M4, W3 |
| Re-registered category changes the button text | Code allows it; not run | M5 |
| Same ID replaces; remove works | Not run | M6 |
| Banner while Codepass is in front | Not run | M7 |
| System Settings URL | Not in Apple's pages | M8 |
| Upgraded Mac starts at "not determined" | Not run | M9 |
| Notarized, hardened build | Not run | M10 |
| Windows without a Start-menu shortcut, unsigned | Microsoft's old page says a shortcut is needed | W1 |
| Windows text cut-off | Not stated | W2 |
| `ms-settings:notifications` | From memory | W4 |
| What `go-toast` `SetAppData` writes | Source not readable | W1 shows the effect |

---

## 8. Manual checks

Rules for every check: use a Mac or a VM **without** the installed AWS SSO Hub or Codepass, or quit it first and accept that both share one notification identity. Use `AWSSSOHUB_SANDBOX=<a new empty folder>` so no real data is read. Use the opt-in variable for real notifications. Write down the result and the OS version.

**M1. Development bundle can notify.**
1. Start the development build (`task dev`) with sandbox mode and real notifications on.
2. Trigger a test notification (the fake portal's session set to end in 1 minute).
3. Expected: macOS asks for permission once. After "Allow", a banner appears.
4. If no prompt and no banner appear: read the app log for the send error. Write it down.

**M2. The button.**
1. With permission allowed, trigger the "sign-in ended" notification.
2. Move the pointer over the banner.
3. Write down: is "重新登录 / Sign in again" a button, or inside "Options"?
4. Open System Settings > Notifications > Codepass. Change "Banners" to "Alerts". Trigger again.
5. Write down how the button looks now, and whether the alert stays on screen.
6. Open Notification Center. Right-click the notification. Write down whether the button is offered there.

**M3. Permission off.**
1. In System Settings > Notifications > Codepass, turn "Allow notifications" off.
2. Open Codepass Settings. Expected: the hint "macOS 没有允许 Codepass 发通知" with "打开系统设置".
3. Trigger a notification. Expected: no banner; an in-app toast instead when the window is visible.
4. Copy the send error from the log into this document.

**M4. Click while the app is closed.**
1. Trigger "sign-in ended". Do not click it.
2. Quit Codepass.
3. Open Notification Center. Press "Sign in again".
4. Expected: Codepass starts, and the sign-in for that portal starts. Write down whether the window appears.
5. Repeat with a click on the notification body. Expected: Codepass starts and shows Accounts at that portal.

**M5. Language change.**
1. Trigger a notification in English. Note the button text.
2. Change the language to 中文 in Settings. Trigger again.
3. Expected: the new notification has "重新登录". Write down what the old one shows.

**M6. Replace and remove.**
1. Trigger the heads-up. Then let the session end.
2. Expected: Notification Center shows one entry for that portal, the "ended" one.
3. Sign in to that portal in the app. Expected: the entry is gone.

**M7. App in front.**
1. Keep the Codepass window focused. Let a session end.
2. Expected: the banner still appears.

**M8. Open System Settings.**
1. Run `open "x-apple.systempreferences:com.apple.Notifications-Settings.extension"` on macOS 15, 26 and 27.
2. Expected: System Settings opens at Notifications. Write down what each version does.

**M9. Upgrade.**
1. On a Mac that ran AWS SSO Hub 0.0.2 and never Codepass, install Codepass.
2. Open System Settings > Notifications. Expected: no entry for the app yet.
3. Sign in to a portal. Expected: the permission prompt appears once.

**M10. Release build.**
1. Repeat M1, M2 and M4 with the notarized release candidate on a clean macOS VM.

**W1. Windows, unsigned, no shortcut.**
1. On a Windows 11 VM, run the portable exe from a folder. No installer, no Start-menu shortcut.
2. Trigger "sign-in ended". Expected: a toast with the name "Codepass" and the button.
3. Open the notification centre. Write down whether the toast is kept there.
4. Repeat on Windows 10 1809.
5. If no toast appears: install with the installer (it creates a Start-menu shortcut) and repeat. Write down the difference.

**W2. Long text.**
1. Use a portal name of 40 characters and five accounts.
2. Write down where Windows cuts the title and the body.

**W3. Click while the app is closed.**
1. Trigger the toast. Quit Codepass. Open the notification centre. Press the button.
2. Expected: Codepass starts and the sign-in starts.
3. Write down what happens if it does not (a second window, nothing, an error).

**W4. Settings link.**
1. Run `start ms-settings:notifications`. Expected: Settings opens at Notifications.
2. Turn notifications off for Codepass. Trigger one. Write down what the app log says.

---

## 9. Sources

Read 2026-11-16.

- Wails 3.1.0 source in the local module cache: `pkg/services/notifications/` (`notifications.go`, `notifications_darwin.go`, `notifications_darwin.m`, `notifications_darwin_shared.go`, `notifications_windows.go`) and `pkg/application/application.go`.
- Repository: `main.go`, `internal/version/version.go`, `build/darwin/Info.dev.plist`, `build/darwin/Taskfile.yml`, `build/config.yml`, `internal/settings/settings.go`; on `main`: `internal/accounts/session.go`, `internal/api/accounts.go`.
- Apple, `UNNotificationCategory`: https://developer.apple.com/documentation/usernotifications/unnotificationcategory
- Apple, "Declaring your actionable notification types": https://developer.apple.com/documentation/usernotifications/declaring-your-actionable-notification-types
- Apple, "Handling notifications and notification-related actions": https://developer.apple.com/documentation/usernotifications/handling-notifications-and-notification-related-actions
- Apple, `UNNotificationActionOptions`: https://developer.apple.com/documentation/usernotifications/unnotificationactionoptions
- Apple, `UNUserNotificationCenterDelegate`: https://developer.apple.com/documentation/usernotifications/unusernotificationcenterdelegate
- Apple, `requestAuthorization(options:completionHandler:)`: https://developer.apple.com/documentation/usernotifications/unusernotificationcenter/requestauthorization(options:completionhandler:)
- Apple, `UNAuthorizationStatus`: https://developer.apple.com/documentation/usernotifications/unauthorizationstatus
- Microsoft, toast schema, `action` element: https://learn.microsoft.com/en-us/uwp/schemas/tiles/toastschema/element-action
- Microsoft, "How to enable desktop toast notifications through an AppUserModelID": https://learn.microsoft.com/en-us/windows/win32/shell/enable-desktop-toast-with-appusermodelid
- `go-toast` package page (fields of `AppData` only): https://pkg.go.dev/git.sr.ht/~jackmordaunt/go-toast/v2/wintoast
- Not reachable on the day: Microsoft's "toast content" and "send a local toast from other apps" pages (404 / 502), and the `go-toast` source host.
