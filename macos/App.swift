import SwiftUI
import AppKit
import UserNotifications

final class AppDelegate: NSObject, NSApplicationDelegate, UNUserNotificationCenterDelegate {
    func applicationDidFinishLaunching(_ notification: Notification) {
        let center = UNUserNotificationCenter.current()
        center.delegate = self
        center.requestAuthorization(options: [.alert, .sound]) { _, _ in }
    }
    // The app keeps watching usage from the menu bar after its window closes.
    func applicationShouldTerminateAfterLastWindowClosed(_ sender: NSApplication) -> Bool { false }
    func userNotificationCenter(_ center: UNUserNotificationCenter, willPresent notification: UNNotification) async -> UNNotificationPresentationOptions { [.banner, .sound] }
    func userNotificationCenter(_ center: UNUserNotificationCenter, didReceive response: UNNotificationResponse) async {
        await MainActor.run {
            NSApp.activate(ignoringOtherApps: true)
            NSApp.windows.first { $0.canBecomeMain }?.makeKeyAndOrderFront(nil)
        }
    }
}

@main struct CCAMDesktop: App {
    @NSApplicationDelegateAdaptor(AppDelegate.self) var delegate
    @StateObject var model = Manager()
    var menuTitle: String {
        guard let window = model.usage[model.source]?.limits?.five_hour, window.known else { return "CCAM" }
        return "\(Int(window.remaining))%"
    }
    var body: some Scene {
        Window("CCAM · 계정 관리", id: "main") { MainView(model: model) }
            .windowStyle(.titleBar)
        MenuBarExtra {
            MenuContent(model: model)
        } label: {
            Label(menuTitle, systemImage: "person.2.badge.key.fill").labelStyle(.titleAndIcon)
        }
        .menuBarExtraStyle(.window)
    }
}
