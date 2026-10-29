import SwiftUI
import AppKit
import UserNotifications

struct Account: Decodable, Identifiable, Hashable {
    var name: String
    var email: String?
    var plan: String?
    var groups: [String]?
    var share: [String: String]?   // item → group, or "local"
    var dir: String?
    var id: String { name }
}
struct UsageWindow: Decodable {
    var percent: Double
    var known: Bool
    var resets_at: String
    var remaining: Double { max(0, min(100, 100 - percent)) }
}
struct Limits: Decodable { var five_hour: UsageWindow; var weekly: UsageWindow }
struct UsageReport: Decodable {
    var name: String; var status: String; var limits: Limits?
    var source: String?; var at: String?; var error: String?
}
struct RunningProcess: Codable, Identifiable {
    var pid: Int; var start: String; var cwd: String
    var session: String?
    var id: Int { pid }
}
struct Conversation: Decodable, Identifiable, Hashable {
    var id: String; var project: String; var cwd: String; var updated: String
    var title: String?; var preview: String?
    var transferable: Bool?; var reason: String?
    var key: String { project + "/" + id }
    var folder: String { cwd.isEmpty ? project : cwd }
    var canCopy: Bool { transferable ?? true }
}
struct ImportCandidate: Decodable, Identifiable {
    var path: String; var name: String; var email: String
    var id: String { path }
}
struct AppFailure: LocalizedError {
    let message: String
    var errorDescription: String? { message }
}
struct AppAlert: Identifiable {
    let id = UUID()
    var title: String; var message: String; var detail: String
}
struct Launch: Identifiable {
    let id = UUID()
    var title: String; var detail: String; var command: String
}

/// Where prepared commands open. "copy" only puts the command on the clipboard.
enum Opener: String, CaseIterable, Identifiable {
    case terminal, iterm, ghostty, copy
    var id: String { rawValue }
    var label: String {
        switch self {
        case .terminal: return "Terminal"
        case .iterm: return "iTerm"
        case .ghostty: return "Ghostty"
        case .copy: return "명령 복사만 (Cursor·VS Code)"
        }
    }
    var bundleID: String? {
        switch self {
        case .terminal: return "com.apple.Terminal"
        case .iterm: return "com.googlecode.iterm2"
        case .ghostty: return "com.mitchellh.ghostty"
        case .copy: return nil
        }
    }
    var appURL: URL? { bundleID.flatMap { NSWorkspace.shared.urlForApplication(withBundleIdentifier: $0) } }
    var installed: Bool { self == .copy || appURL != nil }
}

// No shell is used for backend operations. Output goes to files to avoid pipe
// deadlocks with long error messages. Temporary output is removed on completion.
struct Backend: Sendable {
    var home: String
    var executable: String { Bundle.main.bundleURL.appendingPathComponent("Contents/Resources/ccam").path }
    static var extraPath: String {
        let user = FileManager.default.homeDirectoryForCurrentUser.path
        return "\(user)/.local/bin:/opt/homebrew/bin:/usr/local/bin:/usr/bin:/bin"
    }
    func environment() -> [String: String] {
        var env = ProcessInfo.processInfo.environment
        env["CCAM_HOME"] = home
        env["CCAM_USER_HOME"] = FileManager.default.homeDirectoryForCurrentUser.path
        env["PATH"] = Backend.extraPath + ":" + (env["PATH"] ?? "")
        return env
    }
    func run(_ args: [String], timeout: TimeInterval? = 90) async throws -> String { try await runFull(args, timeout: timeout).out }
    /// Runs off the main actor. Cancelling the calling task stops the CLI.
    /// timeout nil waits for the CLI: a copy must not be cut in half.
    func runFull(_ args: [String], timeout: TimeInterval? = 90) async throws -> (out: String, err: String) {
        let dir = FileManager.default.temporaryDirectory.appendingPathComponent(UUID().uuidString)
        try FileManager.default.createDirectory(at: dir, withIntermediateDirectories: true, attributes: [.posixPermissions: 0o700])
        defer { try? FileManager.default.removeItem(at: dir) }
        let out = dir.appendingPathComponent("out"), err = dir.appendingPathComponent("err")
        FileManager.default.createFile(atPath: out.path, contents: nil, attributes: [.posixPermissions: 0o600])
        FileManager.default.createFile(atPath: err.path, contents: nil, attributes: [.posixPermissions: 0o600])
        let stdout = try FileHandle(forWritingTo: out), stderr = try FileHandle(forWritingTo: err)
        defer { try? stdout.close(); try? stderr.close() }
        let process = Process()
        process.executableURL = URL(fileURLWithPath: executable)
        process.arguments = args
        process.environment = environment()
        process.standardInput = FileHandle.nullDevice
        process.standardOutput = stdout; process.standardError = stderr
        try process.run()
        // The CLI itself bounds network requests. A final watchdog avoids a stuck UI.
        let deadline = timeout.map { Date().addingTimeInterval($0) }
        while process.isRunning {
            if Task.isCancelled { process.terminate(); throw CancellationError() }
            if let deadline, Date() >= deadline {
                process.terminate()
                throw AppFailure(message: "응답 시간이 초과되었습니다. 실행 상태를 확인한 뒤 다시 시도하세요.")
            }
            try? await Task.sleep(nanoseconds: 50_000_000)
        }
        process.waitUntilExit()
        let output = String(decoding: try Data(contentsOf: out), as: UTF8.self)
        let error = String(decoding: try Data(contentsOf: err), as: UTF8.self)
        guard process.terminationStatus == 0 else { throw AppFailure(message: error + "\n" + output) }
        return (output, error)
    }
}

func shellQuote(_ s: String) -> String {
    // Plain words stay readable in copied commands; everything else is quoted.
    if !s.isEmpty && s.range(of: "^[A-Za-z0-9_./:=@%+-]+$", options: .regularExpression) != nil { return s }
    return "'" + s.replacingOccurrences(of: "'", with: "'\\''") + "'"
}
func parseTime(_ text: String?) -> Date? {
    guard let text else { return nil }
    let iso = ISO8601DateFormatter()
    iso.formatOptions = [.withInternetDateTime, .withFractionalSeconds]
    let date = iso.date(from: text) ?? ISO8601DateFormatter().date(from: text)
    guard let date, date.timeIntervalSince1970 > 0 else { return nil }
    return date
}
func shortTime(_ text: String?) -> String {
    guard let date = parseTime(text) else { return "확인 전" }
    return date.formatted(date: .abbreviated, time: .shortened)
}
/// "3시간 후", "5분 전".
func relativeTime(_ date: Date) -> String {
    if abs(date.timeIntervalSinceNow) < 60 { return "방금" }
    let f = RelativeDateTimeFormatter()
    f.locale = Locale(identifier: "ko_KR"); f.unitsStyle = .full
    return f.localizedString(for: date, relativeTo: Date())
}
func sourceLabel(_ source: String?) -> String {
    switch source {
    case "live": return "실시간"
    case "cache": return "저장된 값"
    case "statusline": return "상태줄 기록"
    case "claude": return "Claude Code"
    case nil, "": return "-"
    default: return source!
    }
}
func isFolder(_ path: String) -> Bool {
    var isDirectory: ObjCBool = false
    return path.hasPrefix("/") && FileManager.default.fileExists(atPath: path, isDirectory: &isDirectory) && isDirectory.boolValue
}
let shareNames: [(String, String)] = [("settings", "설정·훅"), ("memory", "CLAUDE.md"), ("extensions", "스킬·명령"), ("mcp", "MCP"), ("history", "대화·메모리")]
/// How a new account shares with the others.
enum ShareChoice: Hashable {
    case none
    case join(String)       // an existing group
    case newGroup(String)   // a new group filled from this account
}
func isValidName(_ name: String) -> Bool {
    name.range(of: "^[a-z0-9][a-z0-9_-]{0,31}$", options: .regularExpression) != nil && Int(name) == nil
}
func nameHint(_ name: String) -> String? {
    if name.isEmpty { return nil }
    if Int(name) != nil { return "숫자로만 된 이름은 쓸 수 없습니다." }
    return isValidName(name) ? nil : "영문 소문자·숫자·하이픈(-)·밑줄(_)만, 32자 이내로 입력하세요."
}

/// Turns the CLI's messages into what to do next. The original text stays in the details.
func friendly(_ raw: String) -> String {
    let rules: [(String, String)] = [
        ("target session already exists", "이 대화는 이미 대상 계정에 있습니다. 대상 계정을 선택하고 대화를 두 번 눌러 이어가세요."),
        ("the running sessions changed", "실행 중인 Claude 목록이 바뀌었습니다. [옮기기…]를 다시 눌러 확인하세요."),
        ("could not confirm that PID", "Claude인지 확인할 수 없는 프로세스가 있어 아무것도 종료하지 않았습니다. 그 터미널에서 /exit 하세요."),
        ("nothing was force-stopped", "20초 안에 끝나지 않아 강제로 끄지 않고 멈췄습니다. 그 터미널에서 /exit 한 뒤 다시 시도하세요."),
        ("may be writing this conversation", "이 대화를 쓰는 Claude가 아직 실행 중입니다. 그 창에서 /exit 한 뒤 다시 시도하세요."),
        ("another session transfer is in progress", "다른 옮기기가 진행 중입니다. 끝난 뒤 다시 시도하세요."),
        ("running; quit Claude Code there first", "그 계정의 Claude가 실행 중이라 공유 설정을 바꿀 수 없습니다. VS Code 확장·터미널의 Claude를 모두 종료한 뒤 다시 시도하세요."),
        ("broken link", "공유 폴더 연결이 끊어졌습니다. 터미널에서 ccam-team doctor 로 확인하세요."),
        ("a group named", "같은 이름의 공유 그룹이 이미 있습니다."),
        ("is running; quit", "이 계정에서 Claude가 아직 실행 중입니다. 작업을 마친 뒤 다시 시도하세요."),
        ("session(s) running in", "이 프로젝트에서 Claude가 아직 실행 중입니다. 작업을 마친 뒤 다시 시도하세요."),
        ("another ccam process is using the login", "다른 작업이 이 계정의 로그인을 쓰고 있습니다. 잠시 후 다시 시도하세요."),
        ("profile names are", "계정 이름은 영문 소문자·숫자·하이픈(-)·밑줄(_)로 32자 이내여야 합니다."),
        ("cannot be all digits", "숫자로만 된 이름은 쓸 수 없습니다."),
        ("is a ccam command", "예약된 단어라 이름으로 쓸 수 없습니다. 다른 이름을 고르세요."),
        ("already exists", "같은 이름의 계정이 이미 있습니다. 다른 이름을 고르세요."),
        ("folder is already registered", "이미 등록된 폴더입니다."),
        ("symlink not supported", "링크로 연결된 프로젝트 폴더의 대화는 복사할 수 없습니다."),
        ("session changed while reading", "옮기는 동안 대화가 바뀌었습니다. Claude를 종료한 뒤 다시 시도하세요."),
        ("source changed", "옮기는 동안 대화가 바뀌었습니다. Claude를 종료한 뒤 다시 시도하세요."),
        ("128 MiB", "대화가 너무 커서(128MB 초과) 옮길 수 없습니다."),
        ("invalid JSONL", "대화 기록 일부가 손상되어 옮길 수 없습니다."),
        ("verification failed", "복사 후 검증에 실패했습니다. 원본은 그대로이고 백업이 남아 있습니다."),
        ("not a folder", "폴더를 찾을 수 없습니다."),
    ]
    for (needle, text) in rules where raw.contains(needle) { return text }
    let first = raw.split(separator: "\n").map { $0.trimmingCharacters(in: .whitespaces) }.first { !$0.isEmpty } ?? ""
    let cleaned = first.hasPrefix("ccam: ") ? String(first.dropFirst(6)) : first
    if cleaned.range(of: "[가-힣]", options: .regularExpression) != nil { return cleaned }
    return "작업을 마치지 못했습니다. 아래 자세한 내용을 확인하세요."
}

@MainActor final class Manager: ObservableObject {
    @Published var accounts: [Account] = []
    @Published var usage: [String: UsageReport] = [:]
    @Published var sessions: [Conversation] = []
    @Published var skipped = 0
    @Published var source = "" {
        didSet {
            guard oldValue != source else { return }
            targetChosen = false; chooseTarget()
            Task { await loadSessions() }
        }
    }
    @Published var target = ""
    @Published var targetChosen = false
    @Published var selectedSession = ""
    @Published var search = ""
    @Published var activity: String?
    @Published var cancellable = false
    @Published var message = ""
    @Published var alert: AppAlert?
    @Published var home: String
    @Published var launch: Launch?
    @Published var addName = "" { didSet { if oldValue != addName { addError = nil } } }
    @Published var addError: String?
    @Published var showAdd = false
    @Published var shareChoice: ShareChoice = .none
    @Published var shareHistory = true
    @Published var showTransfer = false
    @Published var running: [String: [RunningProcess]] = [:]
    @Published var others: [String: [RunningProcess]] = [:]
    @Published var stopOthers = false
    @Published var projectOverride = ""
    @Published var candidates: [ImportCandidate] = []
    @Published var importing: ImportCandidate?
    @Published var importName = "" { didSet { if oldValue != importName { importError = nil } } }
    @Published var importError: String?
    @Published var renaming: String?
    @Published var renameText = "" { didSet { if oldValue != renameText { renameError = nil } } }
    @Published var renameError: String?
    @Published var waitingLogin: String?
    @Published var lastUsageAt: Date?
    @Published var usageNote: String?
    @Published var opener: Opener { didSet { UserDefaults.standard.set(opener.rawValue, forKey: "opener") } }
    @Published var wrapperReady = false
    private var job: Task<Void, Never>?
    private var loginWatch: Task<Void, Never>?
    private var usageLoading = false
    private var sessionGeneration = 0
    private var notified: [String]
    var backend: Backend { Backend(home: home) }
    var selected: Conversation? { sessions.first { $0.key == selectedSession } }
    var busy: Bool { activity != nil }
    let translocated = Bundle.main.bundlePath.contains("/AppTranslocation/")
    let wrapperPath = FileManager.default.homeDirectoryForCurrentUser.appendingPathComponent(".local/bin/ccam-team").path
    static let wrapperMarker = "# Generated by CCAM.app; rewritten on each launch."

    init() {
        home = UserDefaults.standard.string(forKey: "runtimeHome") ?? FileManager.default.homeDirectoryForCurrentUser.appendingPathComponent(".local/share/ccam-beta/runtime").path
        let saved = Opener(rawValue: UserDefaults.standard.string(forKey: "opener") ?? "") ?? .terminal
        opener = saved.installed ? saved : .terminal
        notified = UserDefaults.standard.stringArray(forKey: "notified") ?? []
        Task { await start() }
    }

    func start() async {
        cleanLaunchScripts()
        installWrapper()
        await refresh()
        // Usage is checked in the background so a nearly spent account is noticed in time.
        while !Task.isCancelled {
            try? await Task.sleep(nanoseconds: 300_000_000_000)
            await autoRefresh()
        }
    }

    // MARK: Running work

    /// Runs one user action at a time. The window stays usable; a second action waits its turn.
    func execute(_ label: String, cancellable: Bool = true, onError: ((String) -> Void)? = nil, _ operation: @escaping @MainActor () async throws -> Void) async {
        if let activity {
            message = "\(activity) 중입니다. 끝난 뒤 다시 눌러 주세요."
            return
        }
        activity = label; self.cancellable = cancellable
        let task = Task { @MainActor in
            do { try await operation() }
            catch is CancellationError { self.message = "\(label)을(를) 취소했습니다." }
            catch {
                if let onError { onError(friendly(error.localizedDescription)) }
                else { self.fail(label, error) }
            }
        }
        job = task
        await task.value
        job = nil; activity = nil; self.cancellable = false
    }
    func cancel() { if cancellable { job?.cancel() } }
    func fail(_ label: String, _ error: Error) {
        let raw = error.localizedDescription.trimmingCharacters(in: .whitespacesAndNewlines)
        alert = AppAlert(title: "\(label)에 실패했습니다", message: friendly(raw), detail: raw)
    }

    // MARK: Loading

    func refresh() async {
        await execute("새로고침") {
            try await self.loadAccounts()
            await self.loadCandidates()
            await self.loadUsage()
        }
        await loadSessions()
    }
    func loadAccounts() async throws {
        let raw = try await backend.run(["list", "--json"])
        accounts = try JSONDecoder().decode([Account].self, from: Data(raw.utf8))
        if !accounts.contains(where: { $0.name == source }) { source = accounts.first?.name ?? "" }
        chooseTarget()
    }
    func loadCandidates() async {
        guard let raw = try? await backend.run(["import", "--candidates"]),
              let found = try? JSONDecoder().decode([ImportCandidate].self, from: Data(raw.utf8)) else { return }
        candidates = found
    }
    func loadUsage() async {
        guard !usageLoading else { return }
        usageLoading = true
        defer { usageLoading = false }
        do {
            let raw = try await backend.run(["usage", "--json"])
            let reports = try JSONDecoder().decode([UsageReport].self, from: Data(raw.utf8))
            usage = Dictionary(reports.map { ($0.name, $0) }, uniquingKeysWith: { _, last in last })
            lastUsageAt = Date(); usageNote = nil
            chooseTarget()
            notifyLow()
        } catch is CancellationError {
        } catch {
            usageNote = "사용량을 가져오지 못했습니다. 잠시 후 새로고침하세요."
        }
    }
    func autoRefresh() async {
        guard activity == nil, !accounts.isEmpty else { return }
        await loadUsage()
    }
    /// Called when the app comes forward: values older than a minute are fetched again.
    func refreshIfStale() async {
        if let lastUsageAt, Date().timeIntervalSince(lastUsageAt) < 60 { return }
        await autoRefresh()
    }
    func loadSessions() async {
        sessionGeneration += 1
        let generation = sessionGeneration, account = source
        sessions = []; selectedSession = ""; skipped = 0
        guard !account.isEmpty else { return }
        do {
            let result = try await backend.runFull(["session", "list", account, "--json", "--preview"])
            let found = try JSONDecoder().decode([Conversation].self, from: Data(result.out.utf8))
            guard generation == sessionGeneration && account == source else { return }
            // The CLI speaks English: "You: " is the user's own words.
            sessions = found.map { s in
                var s = s
                if let p = s.preview, p.hasPrefix("You: ") { s.preview = "나: " + p.dropFirst(5) }
                if s.title?.isEmpty == true { s.title = nil }
                return s
            }.sorted { (parseTime($0.updated) ?? .distantPast) > (parseTime($1.updated) ?? .distantPast) }
            skipped = result.err.components(separatedBy: "\n").filter { $0.hasPrefix("Skipped") }.count
        } catch { if generation == sessionGeneration { fail("대화 목록 불러오기", error) } }
    }

    // MARK: Recommendation and alerts

    /// The smaller of the two remaining percentages, when both are known.
    func score(_ name: String) -> Double? {
        guard let r = usage[name], r.status == "ok", let l = r.limits, l.five_hour.known, l.weekly.known else { return nil }
        return min(l.five_hour.remaining, l.weekly.remaining)
    }
    func best(excluding name: String) -> String? {
        accounts.filter { $0.name != name }
            .compactMap { a in score(a.name).map { (a.name, $0) } }
            .max { $0.1 < $1.1 }?.0
    }
    var recommended: String? { best(excluding: source) }
    func chooseTarget() {
        let others = accounts.filter { $0.name != source }
        if targetChosen, others.contains(where: { $0.name == target }) { return }
        target = recommended ?? others.first?.name ?? ""
    }
    func notifyLow() {
        for account in accounts {
            guard let r = usage[account.name], r.status == "ok", let l = r.limits else { continue }
            for (label, w) in [("5시간", l.five_hour), ("주간", l.weekly)] where w.known && w.remaining <= 10 {
                let key = "\(account.name)|\(label)|\(w.resets_at)"
                guard !notified.contains(key) else { continue }
                notified = Array((notified + [key]).suffix(200))
                UserDefaults.standard.set(notified, forKey: "notified")
                let content = UNMutableNotificationContent()
                content.title = "\(account.name) · \(label) 사용량 \(Int(w.remaining))% 남음"
                if let next = best(excluding: account.name), let s = score(next) {
                    content.body = "\(next) 계정은 \(Int(s))% 남아 있습니다. CCAM에서 대화를 옮겨 이어가세요."
                } else {
                    content.body = "초기화: " + (parseTime(w.resets_at).map(relativeTime) ?? "확인 전")
                }
                UNUserNotificationCenter.current().add(UNNotificationRequest(identifier: key, content: content, trigger: nil))
            }
        }
    }

    // MARK: Accounts

    func chooseHome() {
        let panel = NSOpenPanel(); panel.canChooseFiles = false; panel.canChooseDirectories = true
        panel.message = "이전에 쓰던 ccam 데이터 폴더(config·data·state가 들어 있는 폴더)를 선택하세요. ~/.claude 폴더가 아닙니다."
        guard panel.runModal() == .OK, let url = panel.url else { return }
        home = url.path; UserDefaults.standard.set(home, forKey: "runtimeHome")
        accounts = []; sessions = []; usage = [:]; source = ""; target = ""; targetChosen = false
        installWrapper()
        Task { await refresh() }
    }
    /// Both accounts read the same projects folder: a switch copies nothing.
    func sharedHistory(_ a: String, _ b: String) -> Bool {
        guard let x = accounts.first(where: { $0.name == a })?.share?["history"],
              let y = accounts.first(where: { $0.name == b })?.share?["history"] else { return false }
        return x != "local" && x == y
    }
    func shareSummary(_ account: Account) -> String {
        let items = shareNames.filter { (account.share?[$0.0] ?? "local") != "local" }.map { $0.1 }
        return items.isEmpty ? "공유 없음 (이 계정 전용 설정)" : "공유: " + items.joined(separator: " · ")
    }
    var groupNames: [String] { Array(Set(accounts.flatMap { $0.groups ?? [] })).sorted() }
    func openAdd() {
        addError = nil
        shareChoice = groupNames.first.map { .join($0) } ?? accounts.first.map { .newGroup($0.name) } ?? .none
        showAdd = true
    }
    func add() async {
        let name = addName.trimmingCharacters(in: .whitespacesAndNewlines)
        guard isValidName(name) else { addError = nameHint(name); return }
        guard !accounts.contains(where: { $0.name == name }) else { addError = "같은 이름의 계정이 이미 있습니다."; return }
        addError = nil
        var added = false
        let choice = shareChoice, history = shareHistory
        await execute("계정 추가", cancellable: false, onError: { self.addError = $0 }) {
            switch choice {
            case .none:
                _ = try await self.backend.run(["add", name, "--no-group", "--no-login"])
            case .join(let group):
                _ = try await self.backend.run(["add", name, "--group", group, "--no-login"])
            case .newGroup(let owner):
                // The new account first, so a bad name moves nothing; it is
                // empty, so joining the group with the owner conflicts with nothing.
                _ = try await self.backend.run(["add", name, "--no-group", "--no-login"])
                let items = "mcp,settings,memory,extensions" + (history ? ",history" : "")
                var n = 1
                while true {
                    let group = n == 1 ? "shared" : "shared\(n)"
                    do {
                        // The owner's files fill the group; ccam backs up anything replaced.
                        _ = try await self.backend.run(["group", "add", group, "--items", items, "--members", owner + "," + name, "--from", owner, "--yes"], timeout: nil)
                        break
                    } catch {
                        if error.localizedDescription.contains("a group named"), n < 20 { n += 1; continue }
                        // Nothing was shared: take back the empty, never logged in account.
                        _ = try? await self.backend.run(["remove", name, "--yes"])
                        throw error
                    }
                }
            }
            added = true
        }
        guard added else { return }
        showAdd = false; addName = ""
        await refresh()
        source = name
        login(name)
    }
    func login(_ name: String) {
        // Signing in needs a real terminal, even when commands are only copied.
        let via: Opener = opener == .copy || !opener.installed ? .terminal : opener
        do { try open(command(["login", name]), with: via) } catch { fail("로그인 창 열기", error); return }
        message = "브라우저에서 \(name) 계정으로 로그인하세요. 인증 코드는 열린 터미널 창에 입력합니다."
        watchLogin(name)
    }
    /// Notices the finished login by itself, so nobody has to press refresh.
    func watchLogin(_ name: String) {
        loginWatch?.cancel()
        waitingLogin = name
        let hadEmail = accounts.first { $0.name == name }?.email?.isEmpty == false
        loginWatch = Task { @MainActor in
            for round in 1...75 { // about five minutes
                try? await Task.sleep(nanoseconds: 4_000_000_000)
                if Task.isCancelled { return }
                var done = false
                if !hadEmail {
                    if let raw = try? await self.backend.run(["list", "--json"]),
                       let list = try? JSONDecoder().decode([Account].self, from: Data(raw.utf8)) {
                        done = list.first { $0.name == name }?.email?.isEmpty == false
                    }
                } else if round % 4 == 0,
                          let raw = try? await self.backend.run(["usage", "--json", "--no-cache"]),
                          let reports = try? JSONDecoder().decode([UsageReport].self, from: Data(raw.utf8)) {
                    done = reports.first { $0.name == name }?.status == "ok"
                }
                if done {
                    self.waitingLogin = nil
                    self.message = "\(name) 로그인을 확인했습니다."
                    await self.refresh()
                    return
                }
            }
            self.waitingLogin = nil
        }
    }
    func stopWatchingLogin() { loginWatch?.cancel(); waitingLogin = nil }
    func rename() async {
        guard let old = renaming else { return }
        let new = renameText.trimmingCharacters(in: .whitespacesAndNewlines)
        guard isValidName(new) else { renameError = nameHint(new); return }
        var ok = false
        await execute("이름 변경", onError: { self.renameError = $0 }) {
            _ = try await self.backend.run(["rename", old, new])
            ok = true
        }
        guard ok else { return }
        renaming = nil
        if source == old { source = new }
        if target == old { target = new }
        await refresh()
    }
    func logout(_ name: String) async {
        await execute("로그아웃") { _ = try await self.backend.run(["logout", name, "-y"]) }
        await refresh()
    }
    func remove(_ name: String) async {
        await execute("계정 삭제") { _ = try await self.backend.run(["remove", name, "-y"]) }
        await refresh()
    }
    func startImport(_ candidate: ImportCandidate) {
        importing = candidate
        importName = candidate.name.isEmpty ? "main" : candidate.name
        importError = nil
    }
    func importFolder() async {
        guard let candidate = importing else { return }
        let name = importName.trimmingCharacters(in: .whitespacesAndNewlines)
        guard isValidName(name) else { importError = nameHint(name); return }
        var ok = false
        await execute("기존 대화 가져오기", onError: { self.importError = $0 }) {
            _ = try await self.backend.run(["import", candidate.path, "--name", name])
            ok = true
        }
        guard ok else { return }
        importing = nil
        source = name
        await refresh()
        message = "\(candidate.path)의 대화를 \(name) 계정으로 가져왔습니다."
    }

    // MARK: Commands

    /// A short launcher in ~/.local/bin, so copied commands stay readable and
    /// keep working when the app moves (it is rewritten on every launch).
    func installWrapper() {
        wrapperReady = false
        guard !translocated else { return }
        let url = URL(fileURLWithPath: wrapperPath)
        if let existing = try? String(contentsOf: url, encoding: .utf8), !existing.contains(Manager.wrapperMarker) { return }
        let body = "#!/bin/sh\n\(Manager.wrapperMarker)\nexport CCAM_HOME=\(shellQuote(home))\nexport CCAM_USER_HOME=\"$HOME\"\nexport PATH=\(shellQuote(Backend.extraPath)):\"$PATH\"\nexec \(shellQuote(backend.executable)) \"$@\"\n"
        do {
            try FileManager.default.createDirectory(at: url.deletingLastPathComponent(), withIntermediateDirectories: true)
            try body.write(to: url, atomically: true, encoding: .utf8)
            try FileManager.default.setAttributes([.posixPermissions: 0o755], ofItemAtPath: url.path)
            wrapperReady = true
        } catch { wrapperReady = false }
    }
    func command(_ args: [String], cwd: String? = nil) -> String {
        let launch: String
        if wrapperReady {
            launch = "~/.local/bin/ccam-team"
        } else {
            // The user's PATH stays; ccam's folders only go in front.
            launch = "env CCAM_HOME=" + shellQuote(home) + " CCAM_USER_HOME=\"$HOME\" PATH=" + shellQuote(Backend.extraPath) + ":\"$PATH\" " + shellQuote(backend.executable)
        }
        return (cwd.map { "cd " + shellQuote($0) + " && " } ?? "") + launch + " " + args.map(shellQuote).joined(separator: " ")
    }
    var launchFolder: URL { URL(fileURLWithPath: home).appendingPathComponent("desktop-launches") }
    func cleanLaunchScripts() {
        let old = Date().addingTimeInterval(-3600)
        for url in (try? FileManager.default.contentsOfDirectory(at: launchFolder, includingPropertiesForKeys: [.contentModificationDateKey])) ?? [] {
            let date = (try? url.resourceValues(forKeys: [.contentModificationDateKey]))?.contentModificationDate ?? .distantPast
            if url.pathExtension == "command" && date < old { try? FileManager.default.removeItem(at: url) }
        }
    }
    func open(_ command: String, with via: Opener? = nil) throws {
        let via = via ?? opener
        if via == .copy { copy(command); return }
        guard let app = via.appURL else { throw AppFailure(message: "\(via.label) 앱을 찾지 못했습니다. 왼쪽 '열기 방식'에서 다른 앱을 고르세요.") }
        try FileManager.default.createDirectory(at: launchFolder, withIntermediateDirectories: true, attributes: [.posixPermissions: 0o700])
        let script = launchFolder.appendingPathComponent(UUID().uuidString + ".command")
        // The script removes itself once started; it holds no login tokens.
        let body = "#!/bin/sh\nrm -f -- \"$0\"\n" + command + "\nstatus=$?\nprintf '\\n종료 코드: %s. 이 창을 닫아도 됩니다.\\n' \"$status\"\nexit \"$status\"\n"
        try body.write(to: script, atomically: true, encoding: .utf8)
        try FileManager.default.setAttributes([.posixPermissions: 0o700], ofItemAtPath: script.path)
        let report: (Error?) -> Void = { error in
            if let error { Task { @MainActor in self.fail("\(via.label) 열기", error) } }
        }
        if via == .ghostty {
            let config = NSWorkspace.OpenConfiguration()
            config.arguments = ["-e", script.path]; config.createsNewApplicationInstance = true
            NSWorkspace.shared.openApplication(at: app, configuration: config) { _, error in report(error) }
        } else {
            NSWorkspace.shared.open([script], withApplicationAt: app, configuration: NSWorkspace.OpenConfiguration()) { _, error in report(error) }
        }
    }
    func copy(_ command: String) {
        NSPasteboard.general.clearContents(); NSPasteboard.general.setString(command, forType: .string)
        message = "명령을 복사했습니다. Cursor·VS Code의 터미널 탭에 붙여 넣으세요 (Claude 입력창 말고)."
    }
    func projectFolder() -> String? {
        let panel = NSOpenPanel(); panel.canChooseFiles = false; panel.canChooseDirectories = true
        panel.message = "Claude로 작업할 프로젝트 폴더를 선택하세요."
        return panel.runModal() == .OK ? panel.url?.path : nil
    }
    func newConversation() {
        guard !source.isEmpty, let cwd = projectFolder() else { return }
        do {
            try open(command([source], cwd: cwd))
            if opener != .copy { message = "\(source) 계정으로 새 대화를 열었습니다." }
        } catch { fail("새 대화 시작", error) }
    }
    func resume(_ conversation: Conversation? = nil, copyOnly: Bool = false) {
        guard let conversation = conversation ?? selected else { return }
        // A folder that is gone (a removed worktree) is picked again.
        let cwd = isFolder(conversation.cwd) ? conversation.cwd : projectFolder()
        guard let cwd else { return }
        let text = command([source, "--resume", conversation.id], cwd: cwd)
        do {
            try open(text, with: copyOnly ? .copy : nil)
            if !copyOnly && opener != .copy { message = "\(source) 계정으로 대화를 열었습니다." }
        } catch { fail("대화 재개", error) }
    }

    // MARK: Transfer

    func prepareTransfer() async {
        guard let conversation = selected else { return }
        await execute("실행 중인 Claude 확인") {
            self.running = [:]; self.others = [:]; self.stopOthers = false; self.projectOverride = ""
            for account in [self.source, self.target] {
                // Only the sessions that may be writing this conversation must stop
                // (those recording no conversation included); the rest may keep running.
                let all = try JSONDecoder().decode([RunningProcess].self, from: Data(try await self.backend.run(["session", "running", account]).utf8))
                let mine = try JSONDecoder().decode([RunningProcess].self, from: Data(try await self.backend.run(["session", "running", account, "--session", conversation.id]).utf8))
                self.running[account] = mine
                self.others[account] = all.filter { p in !mine.contains { $0.pid == p.pid } }
            }
            self.showTransfer = true
        }
    }
    func transfer() async {
        guard let conversation = selected, !target.isEmpty, source != target else { return }
        let cwd = isFolder(conversation.cwd) ? conversation.cwd : projectOverride
        guard isFolder(cwd) else { return }
        let from = source, to = target
        // Other conversations keep running unless the user chose to stop them too.
        let scoped = !stopOthers
        showTransfer = false
        await execute("대화 옮기기") {
            let args = ["session", "transfer", from, to, conversation.id, "--project-folder", conversation.project]
            // Catch an existing target or invalid files before interrupting work.
            _ = try await self.backend.run(["--dry-run"] + args + ["--inspect-running"])
            for account in [from, to] {
                let expected = (self.running[account] ?? []) + (scoped ? [] : self.others[account] ?? [])
                if expected.isEmpty { continue }
                self.message = "\(account) 계정의 Claude가 종료되기를 기다리는 중…"
                let json = String(decoding: try JSONEncoder().encode(expected), as: UTF8.self)
                _ = try await self.backend.run(["session", "running", account] + (scoped ? ["--session", conversation.id] : []) + ["--stop-expected", json])
            }
            // Copying is quick and must not be cut in half.
            self.cancellable = false
            _ = try await self.backend.run(["--dry-run"] + args)
            let result = try await self.backend.run(args, timeout: nil)
            if result.contains("nothing to copy") {
                self.launch = Launch(title: "\(to) 계정으로 전환했습니다", detail: "두 계정이 대화 내역을 공유하므로 복사하지 않았습니다. 같은 대화를 두 계정에서 동시에 열지 마세요.", command: self.command([to, "--resume", conversation.id], cwd: cwd))
                self.message = "대화 내역 공유 계정으로 전환했습니다."
            } else {
                let backup = result.split(separator: "\n").first { $0.hasPrefix("Backup:") }
                    .map { "백업 위치: " + $0.dropFirst(7).trimmingCharacters(in: .whitespaces) } ?? ""
                self.launch = Launch(title: "\(to) 계정으로 대화를 옮겼습니다", detail: "원본 대화와 두 계정의 로그인은 그대로입니다.\n" + backup, command: self.command([to, "--resume", conversation.id], cwd: cwd))
                self.message = "백업·복사·검증을 마쳤습니다."
            }
            self.source = to
        }
    }
}
