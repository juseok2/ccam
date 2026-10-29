import SwiftUI
import AppKit

struct Account: Decodable, Identifiable, Hashable {
    var name: String
    var email: String?
    var plan: String?
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
    var id: Int { pid }
}
struct Conversation: Decodable, Identifiable, Hashable {
    var id: String; var project: String; var cwd: String; var updated: String
    var title: String?; var preview: String?
    var key: String { project + "/" + id }
}
struct AppFailure: LocalizedError {
    let message: String
    var errorDescription: String? { message }
}

// No shell is used for backend operations. Output goes to files to avoid pipe
// deadlocks with long error messages. Temporary output is removed on completion.
struct Backend: Sendable {
    var home: String
    var executable: String { Bundle.main.bundleURL.appendingPathComponent("Contents/Resources/ccam").path }
    func environment() -> [String: String] {
        var env = ProcessInfo.processInfo.environment
        env["CCAM_HOME"] = home
        let user = FileManager.default.homeDirectoryForCurrentUser.path
        env["PATH"] = "\(user)/.local/bin:/opt/homebrew/bin:/usr/local/bin:/usr/bin:/bin:" + (env["PATH"] ?? "")
        return env
    }
    func run(_ args: [String]) async throws -> String {
        try await Task.detached(priority: .userInitiated) {
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
            let deadline = Date().addingTimeInterval(90)
            while process.isRunning && Date() < deadline { try await Task.sleep(nanoseconds: 50_000_000) }
            if process.isRunning {
                process.terminate()
                throw AppFailure(message: "응답 시간이 초과되었습니다. 실행 상태를 확인한 뒤 다시 시도하세요.")
            }
            process.waitUntilExit()
            let output = String(decoding: try Data(contentsOf: out), as: UTF8.self)
            let error = String(decoding: try Data(contentsOf: err), as: UTF8.self)
            guard process.terminationStatus == 0 else { throw AppFailure(message: error + "\n" + output) }
            return output
        }.value
    }
}

func shellQuote(_ s: String) -> String { "'" + s.replacingOccurrences(of: "'", with: "'\\''") + "'" }
func shortTime(_ text: String?) -> String {
    guard let text else { return "확인 전" }
    let iso = ISO8601DateFormatter()
    iso.formatOptions = [.withInternetDateTime, .withFractionalSeconds]
    let date = iso.date(from: text) ?? ISO8601DateFormatter().date(from: text)
    guard let date, date.timeIntervalSince1970 > 0 else { return "확인 전" }
    return date.formatted(date: .abbreviated, time: .shortened)
}

@MainActor final class Manager: ObservableObject {
    @Published var accounts: [Account] = []
    @Published var usage: [String: UsageReport] = [:]
    @Published var sessions: [Conversation] = []
    @Published var source = ""
    @Published var target = ""
    @Published var selectedSession = ""
    @Published var busy = false
    @Published var message = "계정을 선택하면 저장된 대화를 확인할 수 있습니다."
    @Published var failure: String?
    @Published var home: String
    @Published var readyCommand: String?
    @Published var addName = ""
    @Published var showAdd = false
    @Published var showTransfer = false
    @Published var running: [String: [RunningProcess]] = [:]
    @Published var projectOverride = ""
    private var sessionGeneration = 0
    var backend: Backend { Backend(home: home) }
    var selected: Conversation? { sessions.first { $0.key == selectedSession } }
    init() {
        home = UserDefaults.standard.string(forKey: "runtimeHome") ?? FileManager.default.homeDirectoryForCurrentUser.appendingPathComponent(".local/share/ccam-beta/runtime").path
    }
    func execute(_ operation: () async throws -> Void) async {
        guard !busy else { return }
        busy = true; failure = nil
        defer { busy = false }
        do { try await operation() } catch { failure = error.localizedDescription }
    }
    func refresh() async {
        await execute {
            let raw = try await backend.run(["list", "--json"])
            accounts = try JSONDecoder().decode([Account].self, from: Data(raw.utf8))
            if !accounts.contains(where: {$0.name == source}) { source = accounts.first?.name ?? "" }
            if !accounts.contains(where: {$0.name == target && $0.name != source}) { target = accounts.first(where: {$0.name != source})?.name ?? "" }
            message = "사용량 조회 중… (계정별로 조회에 실패할 수 있습니다)"
            usage = [:]
            do {
                let raw = try await backend.run(["usage", "--json"])
                let reports = try JSONDecoder().decode([UsageReport].self, from: Data(raw.utf8))
                usage = Dictionary(reports.map { ($0.name, $0) }, uniquingKeysWith: { _, last in last })
                message = "조회 완료 · " + Date().formatted(date: .omitted, time: .shortened)
            } catch { message = "사용량 조회 실패. 계정 목록은 표시됩니다."; failure = error.localizedDescription }
        }
        await loadSessions()
    }
    func loadSessions() async {
        sessionGeneration += 1
        let generation = sessionGeneration, account = source
        sessions = []; selectedSession = ""; projectOverride = ""; readyCommand = nil
        guard !account.isEmpty else { return }
        do {
            let raw = try await backend.run(["session", "list", account, "--json", "--preview"])
            let found = try JSONDecoder().decode([Conversation].self, from: Data(raw.utf8))
            guard generation == sessionGeneration && account == source else { return }
            sessions = found.sorted { $0.updated > $1.updated }
        } catch { if generation == sessionGeneration { failure = error.localizedDescription } }
    }
    func chooseHome() {
        let panel = NSOpenPanel(); panel.canChooseFiles = false; panel.canChooseDirectories = true
        panel.message = "config / data / state를 담는 ccam 전용 폴더를 선택하세요. 일반 .claude 폴더가 아닙니다."
        guard panel.runModal() == .OK, let url = panel.url else { return }
        home = url.path; UserDefaults.standard.set(home, forKey: "runtimeHome")
        accounts = []; sessions = []; usage = [:]; source = ""; target = ""; readyCommand = nil
        Task { await refresh() }
    }
    func projectFolder() -> String? {
        let panel = NSOpenPanel(); panel.canChooseFiles = false; panel.canChooseDirectories = true
        panel.message = "Claude를 실행할 프로젝트 폴더를 선택하세요."
        return panel.runModal() == .OK ? panel.url?.path : nil
    }
    func command(_ args: [String], cwd: String? = nil) -> String {
        let env = backend.environment()
        let launch = "env CCAM_HOME=" + shellQuote(home) + " PATH=" + shellQuote(env["PATH"]!) + " " + shellQuote(backend.executable) + " " + args.map(shellQuote).joined(separator: " ")
        return (cwd.map { "cd " + shellQuote($0) + " && " } ?? "") + launch
    }
    func openTerminal(_ command: String) throws {
        let folder = URL(fileURLWithPath: home).appendingPathComponent("desktop-launches")
        try FileManager.default.createDirectory(at: folder, withIntermediateDirectories: true, attributes: [.posixPermissions: 0o700])
        let script = folder.appendingPathComponent(UUID().uuidString + ".command")
        let body = "#!/bin/sh\n# Generated by CCAM; contains no login tokens.\n" + command + "\nstatus=$?\nprintf '\\n종료 코드: %s. 이 창을 닫아도 됩니다.\\n' \"$status\"\nexit \"$status\"\n"
        try body.write(to: script, atomically: true, encoding: .utf8)
        try FileManager.default.setAttributes([.posixPermissions: 0o700], ofItemAtPath: script.path)
        guard let terminal = NSWorkspace.shared.urlForApplication(withBundleIdentifier: "com.apple.Terminal") else { throw AppFailure(message: "Terminal 앱을 찾지 못했습니다. 명령을 복사해 Cursor에서 실행하세요.") }
        NSWorkspace.shared.open([script], withApplicationAt: terminal, configuration: NSWorkspace.OpenConfiguration()) { _, error in
            if let error { Task { @MainActor in self.failure = error.localizedDescription } }
        }
    }
    func login(_ name: String) {
        do { try openTerminal(command(["login", name])); message = "브라우저에서 로그인 후 새로고침하세요. 인증 코드는 열린 Terminal에 입력하세요." }
        catch { failure = error.localizedDescription }
    }
    func add() async {
        let name = addName.trimmingCharacters(in: .whitespacesAndNewlines)
        await execute {
            _ = try await backend.run(["add", name, "--no-group", "--no-login"])
            showAdd = false; addName = ""; login(name)
        }
        await refresh()
    }
    func newConversation() {
        guard !source.isEmpty, let cwd = projectFolder() else { return }
        readyCommand = command([source], cwd: cwd)
        message = "실행 명령을 준비했습니다. Terminal로 열거나 Cursor 터미널에 복사하세요."
    }
    func resume() {
        guard let selected else { return }
        let cwd = selected.cwd.isEmpty ? projectFolder() : selected.cwd
        guard let cwd else { return }
        readyCommand = command([source, "--resume", selected.id], cwd: cwd)
        message = "선택한 대화를 재개할 명령을 준비했습니다."
    }
    func prepareTransfer() async {
        await execute {
            running = [:]
            for account in [source, target] {
                let raw = try await backend.run(["session", "running", account])
                running[account] = try JSONDecoder().decode([RunningProcess].self, from: Data(raw.utf8))
            }
            showTransfer = true
        }
    }
    func transfer() async {
        guard let conversation = selected, !target.isEmpty, source != target else { return }
        let cwd = conversation.cwd.isEmpty ? projectOverride : conversation.cwd
        var isDirectory: ObjCBool = false
        guard FileManager.default.fileExists(atPath: cwd, isDirectory: &isDirectory), isDirectory.boolValue, cwd.hasPrefix("/") else { failure = "원래 프로젝트 폴더를 선택하세요."; return }
        let from = source, to = target
        showTransfer = false; readyCommand = nil
        await execute {
            let args = ["session", "transfer", from, to, conversation.id, "--project-folder", conversation.project]
            // Catch an existing target or invalid files before interrupting work.
            _ = try await backend.run(["--dry-run"] + args + ["--inspect-running"])
            for account in [from, to] {
                message = "\(account)의 실행 세션 종료를 기다리고 있습니다…"
                let expected = try JSONEncoder().encode(running[account] ?? [])
                _ = try await backend.run(["session", "running", account, "--stop-expected", String(decoding: expected, as: UTF8.self)])
            }
            _ = try await backend.run(["--dry-run"] + args)
            let result = try await backend.run(args)
            readyCommand = command([to, "--resume", conversation.id], cwd: cwd)
            message = "백업·복사·검증 완료. 아래에서 재개 방식을 선택하세요.\n" + result
        }
    }
    func copyCommand() {
        guard let readyCommand else { return }
        NSPasteboard.general.clearContents(); NSPasteboard.general.setString(readyCommand, forType: .string)
        message = "Cursor의 새 셸 터미널에 붙여 넣으세요. 실행 중인 Claude 입력창에는 넣지 마세요."
    }
}

struct Meter: View {
    let label: String; let window: UsageWindow?
    var body: some View {
        VStack(alignment: .leading, spacing: 5) {
            HStack {
                Text(label).font(.caption).foregroundStyle(.secondary)
                Spacer()
                if let window, window.known { Text("남음 \(Int(window.remaining))%").font(.caption.bold()) }
                else { Text("조회 불가").font(.caption).foregroundStyle(.secondary) }
            }
            ProgressView(value: window?.known == true ? window!.remaining : 0, total: 100)
                .tint((window?.remaining ?? 0) > 20 ? Color.teal : Color.orange)
            if let window, window.known { Text("초기화 " + shortTime(window.resets_at)).font(.system(size: 10)).foregroundStyle(.secondary) }
        }
    }
}

struct MainView: View {
    @StateObject var model = Manager()
    var body: some View {
        HStack(spacing: 0) {
            VStack(alignment: .leading, spacing: 18) {
                Label("CCAM", systemImage: "person.2.badge.key.fill").font(.title2.bold())
                Text("계정과 대화 관리").foregroundStyle(.secondary)
                Button { model.showAdd = true } label: { Label("계정 추가", systemImage: "plus") }.buttonStyle(.borderedProminent)
                Divider()
                Label("수동 전환", systemImage: "arrow.left.arrow.right").foregroundStyle(.teal)
                Text("터미널에서는 평소처럼 작업하고, 여기서 다음 계정을 선택하세요.").font(.callout).foregroundStyle(.secondary)
                Spacer()
                Text("데이터 폴더").font(.caption.bold())
                Text(model.home).font(.system(size: 10)).foregroundStyle(.secondary).textSelection(.enabled)
                Button("기존 ccam 폴더 연결…") { model.chooseHome() }
                Text("기존 CLI 설정은 유지됩니다. 자동 전환 설정도 연결한 폴더를 따릅니다.").font(.caption).foregroundStyle(.secondary)
                Text("로컬 베타 · macOS").font(.caption).foregroundStyle(.secondary)
            }.padding(24).frame(width: 220).background(.regularMaterial)
            ScrollView {
                VStack(alignment: .leading, spacing: 22) {
                    HStack {
                        VStack(alignment: .leading, spacing: 5) {
                            Text("어떤 계정으로 이어갈까요?").font(.largeTitle.bold())
                            Text("계정을 고르고, 저장된 대화를 안전하게 이어가세요.").foregroundStyle(.secondary)
                        }
                        Spacer()
                        if model.busy { ProgressView().controlSize(.small) }
                        Button { Task { await model.refresh() } } label: { Label("새로고침", systemImage: "arrow.clockwise") }
                    }
                    if model.accounts.isEmpty {
                        VStack(alignment: .leading, spacing: 8) {
                            Text("등록된 계정이 없습니다").font(.title3.bold())
                            Text("계정을 추가하거나 기존 ccam 데이터 폴더를 연결하세요. 로그인은 계정마다 별도로 진행합니다.")
                        }.padding(24).frame(maxWidth: .infinity, alignment: .leading).background(.quaternary, in: RoundedRectangle(cornerRadius: 14))
                    }
                    accountCards
                    VStack(alignment: .leading, spacing: 12) {
                        HStack {
                            Text("\(model.source.isEmpty ? "계정 선택" : model.source)의 저장된 대화").font(.title3.bold())
                            Spacer()
                            Button("새 대화 시작…") { model.newConversation() }.disabled(model.source.isEmpty)
                        }
                        if model.sessions.isEmpty { Text("저장된 대화가 없거나 현재 형식으로 이전할 수 없습니다.").foregroundStyle(.secondary).padding(.vertical, 12) }
                        ForEach(model.sessions, id: \.key) { session in
                            Button {
                                model.selectedSession = session.key; model.readyCommand = nil
                            } label: {
                                HStack {
                                    Image(systemName: model.selectedSession == session.key ? "checkmark.circle.fill" : "circle").foregroundStyle(.teal)
                                    VStack(alignment: .leading, spacing: 4) {
                                        Text(session.title ?? "내용 미리보기가 없는 대화").font(.headline).lineLimit(2)
                                        if let preview = session.preview, !preview.isEmpty {
                                            Text(preview).font(.callout).foregroundStyle(.secondary).lineLimit(3)
                                        }
                                        Text(session.cwd.isEmpty ? session.project : session.cwd).font(.caption).foregroundStyle(.secondary).lineLimit(1).truncationMode(.middle)
                                        Text(String(session.id.prefix(8)) + " · " + shortTime(session.updated)).font(.caption).foregroundStyle(.secondary)
                                    }
                                    Spacer()
                                }.padding(12).background(model.selectedSession == session.key ? Color.teal.opacity(0.08) : Color.clear, in: RoundedRectangle(cornerRadius: 8))
                            }.buttonStyle(.plain)
                        }
                        Divider()
                        HStack {
                            Button("같은 계정으로 재개") { model.resume() }.disabled(model.selected == nil)
                            Spacer()
                            Picker("전환 대상", selection: $model.target) {
                                Text("계정 선택").tag("")
                                ForEach(model.accounts.filter {$0.name != model.source}) { Text($0.name).tag($0.name) }
                            }.frame(width: 210)
                            Button("백업 후 이 계정으로 전환") { Task { await model.prepareTransfer() } }
                                .buttonStyle(.borderedProminent).tint(.teal)
                                .disabled(model.selected == nil || model.target.isEmpty || model.source == model.target)
                        }
                        Text("전환 시 두 계정의 실행 세션을 확인하고 종료를 요청합니다. 종료되지 않으면 전환을 중단합니다.").font(.caption).foregroundStyle(.secondary)
                    }.padding(20).background(Color(nsColor: .controlBackgroundColor), in: RoundedRectangle(cornerRadius: 14))
                    if model.readyCommand != nil {
                        HStack {
                            Label("재개 준비 완료", systemImage: "terminal").font(.headline)
                            Spacer()
                            Button("Cursor용 명령 복사") { model.copyCommand() }
                            Button("Terminal에서 열기") {
                                do { try model.openTerminal(model.readyCommand!) } catch { model.failure = error.localizedDescription }
                            }.buttonStyle(.borderedProminent)
                        }.padding(18).background(Color.teal.opacity(0.1), in: RoundedRectangle(cornerRadius: 12))
                    }
                    Text(model.message).font(.caption).foregroundStyle(.secondary).textSelection(.enabled)
                    Text("남은 비율은 사용량 창 기준이며 남은 토큰 수가 아닙니다. 캐시·조회 실패 시 표시 시각과 상태를 확인하세요.").font(.caption2).foregroundStyle(.secondary)
                }.padding(28)
            }
        }.frame(minWidth: 1050, minHeight: 700)
        .disabled(model.busy)
        .task { await model.refresh() }
        .onChange(of: model.source) { _ in
            model.target = model.accounts.first(where: {$0.name != model.source})?.name ?? ""
            Task { await model.loadSessions() }
        }
        .alert("확인이 필요합니다", isPresented: Binding(get: {model.failure != nil}, set: {if !$0 {model.failure = nil}})) {
            Button("확인") { model.failure = nil }
        } message: { Text(model.failure ?? "") }
        .sheet(isPresented: $model.showAdd) {
            VStack(alignment: .leading, spacing: 18) {
                Text("계정 추가").font(.title2.bold())
                Text("별명을 입력하면 로그인용 Terminal과 브라우저를 엽니다. 로그인 완료 후 앱에서 새로고침하세요.")
                TextField("예: work, personal", text: $model.addName)
                Text("소문자·숫자·하이픈·밑줄, 최대 32자").font(.caption).foregroundStyle(.secondary)
                HStack { Button("취소") { model.showAdd = false }; Spacer(); Button("등록하고 로그인") { Task { await model.add() } }.disabled(model.addName.isEmpty || model.busy) }
            }.padding(28).frame(width: 440)
        }
        .sheet(isPresented: $model.showTransfer) {
            VStack(alignment: .leading, spacing: 18) {
                Text("\(model.source) → \(model.target)").font(.title2.bold())
                Text("두 프로필의 모든 실행 세션에 정상 종료(SIGTERM)를 요청합니다. 다른 프로젝트의 작업도 중단될 수 있습니다. 종료 확인 후 백업·검증·복사를 진행하며, 종료되지 않으면 전환을 중단합니다.")
                ScrollView {
                    VStack(alignment: .leading, spacing: 12) {
                        ForEach([model.source, model.target], id: \.self) { account in
                            Text("\(account) · 실행 \(model.running[account]?.count ?? 0)개").font(.headline)
                            ForEach(model.running[account] ?? []) { process in
                                Text("PID \(process.pid) · \(process.cwd.isEmpty ? "프로젝트 경로 확인 불가" : process.cwd)").font(.caption).lineLimit(2)
                            }
                        }
                    }.frame(maxWidth: .infinity, alignment: .leading)
                }.frame(maxHeight: 180)
                Text("연결한 ccam 폴더의 프로필 기준입니다. 별도 설치나 다른 프로필에서 같은 이메일로 로그인한 세션은 포함하지 않습니다.").font(.caption).foregroundStyle(.secondary)
                if model.selected?.cwd.isEmpty == true {
                    Button("원래 프로젝트 폴더 선택…") { model.projectOverride = model.projectFolder() ?? "" }
                    Text(model.projectOverride).font(.caption)
                }
                HStack { Button("취소") { model.showTransfer = false }; Spacer(); Button("세션 종료 후 백업 및 전환") { Task { await model.transfer() } }.buttonStyle(.borderedProminent) }
            }.padding(28).frame(width: 460)
        }
    }
    var accountCards: some View {
                    LazyVGrid(columns: [GridItem(.adaptive(minimum: 230))], spacing: 12) {
                        ForEach(model.accounts) { account in
                            accountCard(account)
                        }
                    }
    }
    func accountCard(_ account: Account) -> some View {
        let report = model.usage[account.name]
        let chosen = model.source == account.name
        let background = chosen ? Color.teal.opacity(0.08) : Color(nsColor: .controlBackgroundColor)
        let border = chosen ? Color.teal : Color.gray.opacity(0.2)
        return VStack(alignment: .leading, spacing: 13) {
            accountHeader(account)
            Text(account.email?.isEmpty == false ? account.email! : "로그인이 필요합니다").font(.caption).foregroundStyle(Color.secondary).lineLimit(1)
            Meter(label: "5시간", window: report?.limits?.five_hour)
            Meter(label: "주간", window: report?.limits?.weekly)
            accountFooter(account)
        }
        .padding(16)
        .background(background, in: RoundedRectangle(cornerRadius: 14))
        .overlay(RoundedRectangle(cornerRadius: 14).stroke(border))
    }
    func accountHeader(_ account: Account) -> some View {
        HStack {
            Button { model.source = account.name } label: {
                Label(account.name, systemImage: model.source == account.name ? "checkmark.circle.fill" : "person.crop.circle").font(.headline)
            }.buttonStyle(.plain)
            Spacer()
            Text(account.plan ?? "계정").font(.caption).foregroundStyle(Color.secondary)
        }
    }
    func accountFooter(_ account: Account) -> some View {
        let report = model.usage[account.name]
        let stamp = "확인 " + shortTime(report?.at) + " · " + (report?.source ?? "없음")
        return VStack(alignment: .leading, spacing: 8) {
            HStack {
                Text(status(report)).font(.caption).foregroundStyle(report?.status == "ok" ? Color.secondary : Color.orange)
                Spacer()
                Button("로그인") { model.login(account.name) }.controlSize(.small)
            }
            Text(stamp).font(.system(size: 10)).foregroundStyle(Color.secondary)
        }
    }
    func status(_ report: UsageReport?) -> String {
        guard let report else { return "조회 전" }
        switch report.status {
        case "ok": return report.source == "cache" ? "캐시 정보" : "조회 정상"
        case "expired": return "인증 만료 · 재로그인 필요"
        case "logged_out": return "로그인 필요"
        case "api": return "API 계정 · 사용량 표시 미지원"
        default: return "조회 실패 · 이전 값일 수 있음"
        }
    }
}

@main struct CCAMDesktop: App {
    var body: some Scene {
        WindowGroup("CCAM · 계정 관리") { MainView() }
            .windowStyle(.titleBar)
    }
}
