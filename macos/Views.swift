import SwiftUI
import AppKit

struct Meter: View {
    let label: String; let window: UsageWindow?
    var body: some View {
        VStack(alignment: .leading, spacing: 5) {
            HStack {
                Text(label).font(.caption).foregroundStyle(.secondary)
                Spacer()
                if let window, window.known { Text("\(Int(window.remaining))% 남음").font(.caption.bold()) }
                else { Text("확인 불가").font(.caption).foregroundStyle(.secondary) }
            }
            if let window, window.known {
                ProgressView(value: window.remaining, total: 100)
                    .tint(window.remaining > 20 ? Color.teal : Color.orange)
                if let reset = parseTime(window.resets_at) {
                    Text("초기화 \(relativeTime(reset)) · \(reset.formatted(date: .omitted, time: .shortened))")
                        .font(.caption2).foregroundStyle(.secondary)
                }
            } else {
                // An unknown value is not drawn as an empty (spent) bar.
                Capsule().strokeBorder(style: StrokeStyle(lineWidth: 1, dash: [3]))
                    .foregroundStyle(.tertiary).frame(height: 6)
            }
        }
    }
}

struct Banner: View {
    let icon: String; let color: Color; let text: String
    var body: some View {
        HStack(alignment: .top, spacing: 10) {
            Image(systemName: icon).foregroundStyle(color)
            Text(text).font(.callout).fixedSize(horizontal: false, vertical: true)
            Spacer(minLength: 0)
        }
        .padding(12).background(color.opacity(0.1), in: RoundedRectangle(cornerRadius: 10))
    }
}

struct MainView: View {
    @ObservedObject var model: Manager
    @FocusState private var searchFocused: Bool
    @State private var confirm: (kind: String, name: String)?

    var body: some View {
        HStack(spacing: 0) {
            sidebar
            Divider()
            VStack(spacing: 0) {
                ScrollView {
                    VStack(alignment: .leading, spacing: 18) {
                        header
                        banners
                        if model.accounts.isEmpty { emptyAccounts }
                        LazyVGrid(columns: [GridItem(.adaptive(minimum: 220))], spacing: 12) {
                            ForEach(model.accounts) { accountCard($0) }
                        }
                        conversations
                    }.padding(24)
                }
                Divider()
                actionBar
            }
        }
        .frame(minWidth: 820, minHeight: 560)
        .onReceive(NotificationCenter.default.publisher(for: NSApplication.didBecomeActiveNotification)) { _ in
            Task { await model.refreshIfStale() }
        }
        .sheet(item: $model.alert) { alert in errorSheet(alert) }
        .sheet(isPresented: $model.showAdd) { addSheet }
        .sheet(isPresented: $model.showTransfer) { transferSheet }
        .sheet(item: $model.launch) { launch in launchSheet(launch) }
        .sheet(item: $model.importing) { candidate in importSheet(candidate) }
        .sheet(isPresented: Binding(get: { model.renaming != nil }, set: { if !$0 { model.renaming = nil } })) { renameSheet }
        .confirmationDialog(confirmTitle, isPresented: Binding(get: { confirm != nil }, set: { if !$0 { confirm = nil } }), titleVisibility: .visible) {
            if let confirm {
                Button(confirm.kind == "remove" ? "삭제" : "로그아웃", role: .destructive) {
                    Task { confirm.kind == "remove" ? await model.remove(confirm.name) : await model.logout(confirm.name) }
                }
            }
            Button("취소", role: .cancel) {}
        } message: {
            Text(confirm?.kind == "remove"
                 ? "로그아웃한 뒤 계정 폴더를 휴지통으로 옮깁니다. 대화가 필요하면 먼저 다른 계정으로 옮기세요."
                 : "이 계정의 로그인과 MCP 서버 로그인을 지웁니다. 대화 기록은 남습니다.")
        }
    }

    var confirmTitle: String {
        guard let confirm else { return "" }
        return confirm.kind == "remove" ? "\(confirm.name) 계정을 삭제할까요?" : "\(confirm.name) 계정에서 로그아웃할까요?"
    }

    // MARK: Sidebar

    var sidebar: some View {
        VStack(alignment: .leading, spacing: 14) {
            Label("CCAM", systemImage: "person.2.badge.key.fill").font(.title2.bold())
            Text("Claude 계정과 대화 관리").font(.callout).foregroundStyle(.secondary)
            Button { model.addError = nil; model.showAdd = true } label: { Label("계정 추가", systemImage: "plus") }
                .buttonStyle(.borderedProminent)
            ForEach(model.candidates) { candidate in
                Button { model.startImport(candidate) } label: { Label("기존 대화 가져오기", systemImage: "tray.and.arrow.down") }
                    .help(candidate.path)
            }
            Divider()
            Text("대화 여는 곳").font(.caption.bold())
            Picker("", selection: $model.opener) {
                ForEach(Opener.allCases.filter(\.installed)) { Text($0.label).tag($0) }
            }.labelsHidden()
            Text("작업은 터미널에서 평소처럼 하고, 사용량이 떨어지면 여기서 계정을 바꿉니다.")
                .font(.caption).foregroundStyle(.secondary).fixedSize(horizontal: false, vertical: true)
            Spacer()
            Text("데이터 폴더").font(.caption.bold())
            Text(model.home.replacingOccurrences(of: FileManager.default.homeDirectoryForCurrentUser.path, with: "~"))
                .font(.caption).foregroundStyle(.secondary).lineLimit(2).truncationMode(.middle).textSelection(.enabled)
            Button("다른 데이터 폴더 연결…") { model.chooseHome() }.controlSize(.small)
            Text("베타 \(Bundle.main.object(forInfoDictionaryKey: "CFBundleShortVersionString") as? String ?? "")")
                .font(.caption).foregroundStyle(.secondary)
        }.padding(20).frame(width: 210).background(.regularMaterial)
    }

    // MARK: Header and banners

    var header: some View {
        HStack(alignment: .firstTextBaseline) {
            VStack(alignment: .leading, spacing: 4) {
                Text("어떤 계정으로 이어갈까요?").font(.title.bold())
                Group {
                    if let at = model.lastUsageAt { Text("사용량 확인 \(relativeTime(at)) · 5분마다 자동 확인") }
                    else { Text("계정을 고르고, 저장된 대화를 이어가세요.") }
                }.font(.callout).foregroundStyle(.secondary)
            }
            Spacer()
            Button { Task { await model.refresh() } } label: { Label("새로고침", systemImage: "arrow.clockwise") }
                .keyboardShortcut("r").disabled(model.busy)
        }
    }

    @ViewBuilder var banners: some View {
        if model.translocated {
            Banner(icon: "exclamationmark.triangle.fill", color: .orange,
                   text: "압축을 푼 폴더에서 바로 실행 중입니다. 이 상태로 만든 명령은 나중에 동작하지 않습니다. Finder에서 CCAM을 '응용 프로그램' 폴더로 옮긴 뒤 다시 여세요.")
        }
        if let name = model.waitingLogin {
            HStack {
                ProgressView().controlSize(.small)
                Text("\(name) 로그인을 기다리는 중… 브라우저에서 로그인을 마치면 자동으로 반영됩니다.").font(.callout)
                Spacer()
                Button("그만 기다리기") { model.stopWatchingLogin() }.controlSize(.small)
            }.padding(12).background(Color.teal.opacity(0.1), in: RoundedRectangle(cornerRadius: 10))
        }
        if let note = model.usageNote { Banner(icon: "wifi.exclamationmark", color: .orange, text: note) }
        if !model.candidates.isEmpty && !model.accounts.isEmpty {
            HStack {
                Image(systemName: "tray.and.arrow.down").foregroundStyle(.teal)
                Text("평소 `claude`로 하던 대화 폴더(\(model.candidates.map { $0.path.replacingOccurrences(of: FileManager.default.homeDirectoryForCurrentUser.path, with: "~") }.joined(separator: ", ")))도 가져올 수 있습니다.")
                    .font(.callout)
                Spacer()
                Button("가져오기") { model.startImport(model.candidates[0]) }.controlSize(.small)
            }.padding(12).background(Color.teal.opacity(0.08), in: RoundedRectangle(cornerRadius: 10))
        }
    }

    var emptyAccounts: some View {
        VStack(alignment: .leading, spacing: 10) {
            Text("등록된 계정이 없습니다").font(.title3.bold())
            Text("[계정 추가]로 Claude 계정을 등록하세요. 계정마다 한 번씩 로그인합니다.")
            if let candidate = model.candidates.first {
                Text("평소 `claude`로 하던 대화가 \(candidate.path)에 있습니다. 가져오면 그 대화를 바로 이어갈 수 있습니다.")
                Button("기존 대화 가져오기") { model.startImport(candidate) }.buttonStyle(.borderedProminent).tint(.teal)
            }
        }.padding(20).frame(maxWidth: .infinity, alignment: .leading).background(.quaternary, in: RoundedRectangle(cornerRadius: 14))
    }

    // MARK: Account cards

    func needsLogin(_ account: Account) -> Bool {
        let status = model.usage[account.name]?.status
        return account.email?.isEmpty != false || status == "expired" || status == "logged_out"
    }

    func accountCard(_ account: Account) -> some View {
        let report = model.usage[account.name]
        let chosen = model.source == account.name
        return VStack(alignment: .leading, spacing: 12) {
            HStack {
                Image(systemName: chosen ? "checkmark.circle.fill" : "person.crop.circle").foregroundStyle(chosen ? Color.teal : Color.secondary)
                Text(account.name).font(.headline)
                if model.recommended == account.name {
                    Text("추천").font(.caption2.bold()).padding(.horizontal, 6).padding(.vertical, 2)
                        .background(Color.teal.opacity(0.18), in: Capsule())
                }
                Spacer()
                Text(account.plan ?? "").font(.caption).foregroundStyle(.secondary)
                Menu {
                    Button("다시 로그인") { model.login(account.name) }
                    Button("이름 변경…") { model.renameText = account.name; model.renameError = nil; model.renaming = account.name }
                    Divider()
                    Button("로그아웃…") { confirm = ("logout", account.name) }
                    Button("계정 삭제…") { confirm = ("remove", account.name) }
                } label: { Image(systemName: "ellipsis.circle") }
                    .menuStyle(.borderlessButton).menuIndicator(.hidden).fixedSize().disabled(model.busy)
            }
            Text(account.email?.isEmpty == false ? account.email! : "로그인이 필요합니다").font(.caption).foregroundStyle(.secondary).lineLimit(1)
            Meter(label: "5시간", window: report?.limits?.five_hour)
            Meter(label: "주간", window: report?.limits?.weekly)
            HStack {
                Text(status(report)).font(.caption).foregroundStyle(report?.status == "ok" || report == nil ? Color.secondary : Color.orange)
                Spacer()
                if needsLogin(account) {
                    Button("로그인") { model.login(account.name) }.controlSize(.small).buttonStyle(.borderedProminent).tint(.orange)
                }
            }
        }
        .padding(14)
        .background(chosen ? Color.teal.opacity(0.08) : Color(nsColor: .controlBackgroundColor), in: RoundedRectangle(cornerRadius: 14))
        .overlay(RoundedRectangle(cornerRadius: 14).stroke(chosen ? Color.teal : Color.gray.opacity(0.2)))
        .contentShape(RoundedRectangle(cornerRadius: 14))
        .onTapGesture { model.source = account.name }
        .help("눌러서 이 계정의 대화 보기")
        .accessibilityAddTraits(.isButton)
        .accessibilityAction { model.source = account.name }
    }

    func status(_ report: UsageReport?) -> String {
        guard let report else { return "확인 전" }
        switch report.status {
        case "ok": return sourceLabel(report.source) + " · " + (parseTime(report.at).map(relativeTime) ?? "방금")
        case "expired": return "로그인 만료 · 다시 로그인하세요"
        case "logged_out": return "로그인 필요"
        case "api": return "API 계정 · 사용량 표시 안 됨"
        default: return "확인 실패 · 이전 값일 수 있음"
        }
    }

    // MARK: Conversations

    var filtered: [Conversation] {
        let q = model.search.trimmingCharacters(in: .whitespaces).lowercased()
        guard !q.isEmpty else { return model.sessions }
        return model.sessions.filter { s in
            [s.title, s.preview, s.folder].compactMap { $0?.lowercased() }.contains { $0.contains(q) }
        }
    }
    /// Projects in the order of their latest conversation.
    var groups: [(folder: String, items: [Conversation])] {
        var order: [String] = [], map: [String: [Conversation]] = [:]
        for s in filtered {
            if map[s.folder] == nil { order.append(s.folder) }
            map[s.folder, default: []].append(s)
        }
        return order.map { ($0, map[$0]!) }
    }

    var conversations: some View {
        VStack(alignment: .leading, spacing: 12) {
            HStack {
                Text(model.source.isEmpty ? "대화" : "\(model.source)의 대화").font(.title3.bold())
                Spacer()
                TextField("제목·내용·폴더 검색 (⌘F)", text: $model.search)
                    .textFieldStyle(.roundedBorder).frame(maxWidth: 240).focused($searchFocused)
                Button("") { searchFocused = true }.keyboardShortcut("f").hidden().frame(width: 0)
                Button { model.newConversation() } label: { Label("새 대화", systemImage: "plus.bubble") }
                    .keyboardShortcut("n").disabled(model.source.isEmpty)
            }
            if model.source.isEmpty {
                Text("위에서 계정을 선택하세요.").foregroundStyle(.secondary).padding(.vertical, 8)
            } else if filtered.isEmpty {
                Text(emptyText).foregroundStyle(.secondary).padding(.vertical, 8)
            }
            LazyVStack(alignment: .leading, spacing: 4) {
                ForEach(groups, id: \.folder) { group in
                    HStack(spacing: 6) {
                        Image(systemName: "folder").font(.caption)
                        Text(URL(fileURLWithPath: group.folder).lastPathComponent).font(.caption.bold())
                        Text(group.folder.replacingOccurrences(of: FileManager.default.homeDirectoryForCurrentUser.path, with: "~"))
                            .font(.caption).lineLimit(1).truncationMode(.middle)
                    }.foregroundStyle(.secondary).padding(.top, 10)
                    ForEach(group.items, id: \.key) { row($0) }
                }
            }
            if !model.sessions.isEmpty {
                Text("대화를 한 번 누르면 선택, 두 번 누르면 바로 열립니다.").font(.caption).foregroundStyle(.secondary)
            }
        }.padding(18).background(Color(nsColor: .controlBackgroundColor), in: RoundedRectangle(cornerRadius: 14))
    }

    var emptyText: String {
        if !model.search.isEmpty { return "검색 결과가 없습니다." }
        if model.skipped > 0 { return "옮길 수 있는 대화가 없습니다. 형식이 맞지 않는 대화 \(model.skipped)개는 목록에서 뺐습니다." }
        return "아직 저장된 대화가 없습니다. [새 대화]로 시작하세요."
    }

    func row(_ session: Conversation) -> some View {
        let chosen = model.selectedSession == session.key
        return HStack(alignment: .top) {
            Image(systemName: chosen ? "checkmark.circle.fill" : "circle").foregroundStyle(.teal)
            VStack(alignment: .leading, spacing: 3) {
                Text(session.title ?? "제목 없는 대화").font(.headline).lineLimit(2)
                if let preview = session.preview, !preview.isEmpty {
                    Text(preview).font(.callout).foregroundStyle(.secondary).lineLimit(2)
                }
                Text(parseTime(session.updated).map(relativeTime) ?? "").font(.caption).foregroundStyle(.secondary)
            }
            Spacer()
        }
        .padding(10)
        .background(chosen ? Color.teal.opacity(0.1) : Color.clear, in: RoundedRectangle(cornerRadius: 8))
        .contentShape(Rectangle())
        .onTapGesture(count: 2) { model.selectedSession = session.key; model.resume(session) }
        .onTapGesture { model.selectedSession = session.key }
        .help("\(session.folder)\n대화 ID \(session.id)\n수정 \(shortTime(session.updated))")
        .accessibilityElement(children: .combine)
        .accessibilityAddTraits(chosen ? [.isButton, .isSelected] : .isButton)
        .accessibilityAction { model.selectedSession = session.key }
        .accessibilityAction(named: "열기") { model.selectedSession = session.key; model.resume(session) }
    }

    // MARK: Bottom bar (always visible)

    var actionBar: some View {
        VStack(alignment: .leading, spacing: 8) {
            if let selected = model.selected {
                HStack(spacing: 10) {
                    Text(selected.title ?? "선택한 대화").font(.callout.bold()).lineLimit(1).truncationMode(.tail)
                        .frame(minWidth: 80, alignment: .leading).layoutPriority(-1)
                    Spacer(minLength: 8)
                    Button("이어가기") { model.resume() }
                        .help("\(model.source) 계정으로 \(model.opener == .copy ? "명령 복사" : model.opener.label + "에서 열기")")
                    if model.opener != .copy { Button("명령 복사") { model.resume(copyOnly: true) } }
                    if model.accounts.count < 2 {
                        Text("다른 계정이 있어야 옮길 수 있습니다").font(.caption).foregroundStyle(.secondary)
                        Button("계정 추가") { model.addError = nil; model.showAdd = true }
                    } else {
                        Divider().frame(height: 18)
                        Image(systemName: "arrow.right").foregroundStyle(.secondary).help("다른 계정으로 옮기기")
                        Picker("옮길 계정", selection: Binding(get: { model.target }, set: { model.target = $0; model.targetChosen = true })) {
                            ForEach(model.accounts.filter { $0.name != model.source }) { a in
                                Text(a.name + (model.score(a.name).map { " · \(Int($0))%" } ?? "") + (model.recommended == a.name ? " (추천)" : "")).tag(a.name)
                            }
                        }.labelsHidden().frame(maxWidth: 190)
                        Button("옮기기…") { Task { await model.prepareTransfer() } }
                            .buttonStyle(.borderedProminent).tint(.teal)
                            .disabled(model.busy || model.target.isEmpty || model.source == model.target)
                    }
                }
            }
            HStack(spacing: 8) {
                if let activity = model.activity {
                    ProgressView().controlSize(.small)
                    Text("\(activity) 중…").font(.caption)
                    if model.cancellable { Button("취소") { model.cancel() }.controlSize(.small) }
                } else {
                    Text(model.message).font(.caption).foregroundStyle(.secondary).lineLimit(2).textSelection(.enabled)
                }
                Spacer()
                Text("남은 비율은 사용 한도 기준이며 토큰 수가 아닙니다.").font(.caption2).foregroundStyle(.tertiary)
            }
        }.padding(.horizontal, 20).padding(.vertical, 12).background(.bar)
    }

    // MARK: Sheets

    func errorSheet(_ alert: AppAlert) -> some View {
        VStack(alignment: .leading, spacing: 14) {
            Label(alert.title, systemImage: "exclamationmark.triangle.fill").font(.title3.bold()).foregroundStyle(.orange)
            Text(alert.message).fixedSize(horizontal: false, vertical: true)
            if !alert.detail.isEmpty && alert.detail != alert.message {
                DisclosureGroup("자세한 내용 (문제 제보 시 첨부)") {
                    ScrollView {
                        Text(alert.detail).font(.system(.caption, design: .monospaced)).textSelection(.enabled)
                            .frame(maxWidth: .infinity, alignment: .leading)
                    }.frame(maxHeight: 160)
                }
            }
            HStack { Spacer(); Button("확인") { model.alert = nil }.keyboardShortcut(.defaultAction) }
        }.padding(24).frame(width: 460)
    }

    func nameField(_ text: Binding<String>, error: String?, submit: @escaping () -> Void) -> some View {
        VStack(alignment: .leading, spacing: 6) {
            TextField("예: work, personal", text: text).textFieldStyle(.roundedBorder).onSubmit(submit)
            if let hint = error ?? nameHint(text.wrappedValue) {
                Text(hint).font(.caption).foregroundStyle(.red)
            } else {
                Text("영문 소문자·숫자·하이픈(-)·밑줄(_), 32자 이내").font(.caption).foregroundStyle(.secondary)
            }
        }
    }

    var addSheet: some View {
        VStack(alignment: .leading, spacing: 16) {
            Text("계정 추가").font(.title2.bold())
            Text("구분할 이름을 정하면 로그인 창이 열립니다. 브라우저에서 로그인을 마치면 앱에 자동으로 반영됩니다.")
                .fixedSize(horizontal: false, vertical: true)
            nameField($model.addName, error: model.addError) { Task { await model.add() } }
            HStack {
                Button("취소") { model.showAdd = false }.keyboardShortcut(.cancelAction)
                Spacer()
                Button("등록하고 로그인") { Task { await model.add() } }
                    .keyboardShortcut(.defaultAction)
                    .disabled(!isValidName(model.addName) || model.busy)
            }
        }.padding(26).frame(width: 440)
    }

    func importSheet(_ candidate: ImportCandidate) -> some View {
        VStack(alignment: .leading, spacing: 16) {
            Text("기존 대화 가져오기").font(.title2.bold())
            Text("\(candidate.path)\(candidate.email.isEmpty ? "" : " (\(candidate.email))")에 있는 대화를 계정으로 등록합니다.")
                .fixedSize(horizontal: false, vertical: true)
            Text("폴더를 옮기거나 복사하지 않고 그 자리에서 등록합니다. 평소 `claude`와 같은 로그인·대화를 함께 쓰며, 홈 폴더의 .claude.json을 이 폴더에 연결합니다.")
                .font(.callout).foregroundStyle(.secondary).fixedSize(horizontal: false, vertical: true)
            nameField($model.importName, error: model.importError) { Task { await model.importFolder() } }
            HStack {
                Button("취소") { model.importing = nil }.keyboardShortcut(.cancelAction)
                Spacer()
                Button("가져오기") { Task { await model.importFolder() } }
                    .keyboardShortcut(.defaultAction).disabled(!isValidName(model.importName) || model.busy)
            }
        }.padding(26).frame(width: 460)
    }

    var renameSheet: some View {
        VStack(alignment: .leading, spacing: 16) {
            Text("\(model.renaming ?? "") 이름 변경").font(.title2.bold())
            nameField($model.renameText, error: model.renameError) { Task { await model.rename() } }
            HStack {
                Button("취소") { model.renaming = nil }.keyboardShortcut(.cancelAction)
                Spacer()
                Button("변경") { Task { await model.rename() } }
                    .keyboardShortcut(.defaultAction).disabled(!isValidName(model.renameText) || model.busy)
            }
        }.padding(26).frame(width: 400)
    }

    func processList(_ items: [RunningProcess]) -> some View {
        ForEach(items) { process in
            Text("· " + (process.cwd.isEmpty ? "폴더를 알 수 없는 Claude" : process.cwd.replacingOccurrences(of: FileManager.default.homeDirectoryForCurrentUser.path, with: "~")))
                .font(.caption).lineLimit(1).truncationMode(.middle).help("PID \(process.pid)")
        }
    }

    var transferSheet: some View {
        let accounts = [model.source, model.target]
        let unknownFolder = model.selected?.cwd.isEmpty == true
        let mustStop = accounts.reduce(0) { $0 + (model.running[$1]?.count ?? 0) }
        let otherCount = accounts.reduce(0) { $0 + (model.others[$1]?.count ?? 0) }
        return VStack(alignment: .leading, spacing: 14) {
            Text("\(model.source) → \(model.target)로 대화 옮기기").font(.title2.bold())
            Text("대화를 백업한 뒤 \(model.target) 계정으로 복사합니다. 원본 대화와 두 계정의 로그인은 그대로 남습니다.")
                .fixedSize(horizontal: false, vertical: true)
            ScrollView {
                VStack(alignment: .leading, spacing: 8) {
                    if mustStop == 0 {
                        Label("이 프로젝트에서 실행 중인 Claude가 없습니다. 바로 옮길 수 있습니다.", systemImage: "checkmark.circle").foregroundStyle(.teal)
                    } else {
                        Text(unknownFolder ? "두 계정에서 실행 중인 Claude \(mustStop)개를 종료합니다" : "이 프로젝트에서 실행 중인 Claude \(mustStop)개를 종료합니다").font(.headline)
                        ForEach(accounts, id: \.self) { account in
                            if let items = model.running[account], !items.isEmpty {
                                Text(account).font(.caption.bold())
                                processList(items)
                            }
                        }
                    }
                    if otherCount > 0 {
                        Divider()
                        Text("다른 프로젝트에서 실행 중 \(otherCount)개").font(.headline)
                        ForEach(accounts, id: \.self) { account in processList(model.others[account] ?? []) }
                        Toggle("다른 프로젝트의 Claude도 함께 종료", isOn: $model.stopOthers)
                        Text(model.stopOthers ? "모두 종료한 뒤 옮깁니다." : "그대로 두고 이 프로젝트의 Claude만 종료합니다.")
                            .font(.caption).foregroundStyle(.secondary)
                    }
                }.frame(maxWidth: .infinity, alignment: .leading)
            }.frame(maxHeight: 220)
            if unknownFolder {
                HStack {
                    Button("원래 프로젝트 폴더 선택…") { model.projectOverride = model.projectFolder() ?? "" }
                    Text(model.projectOverride.isEmpty ? "대화 기록에 폴더가 없어 직접 골라야 합니다." : model.projectOverride)
                        .font(.caption).foregroundStyle(.secondary).lineLimit(1).truncationMode(.middle)
                }
            }
            if mustStop + (model.stopOthers ? otherCount : 0) > 0 {
                Text("종료를 요청하고 20초 안에 끝나지 않으면 강제로 끄지 않고 멈춥니다. 응답을 받는 중이었다면 그 응답은 끊깁니다.")
                    .font(.caption).foregroundStyle(.secondary).fixedSize(horizontal: false, vertical: true)
            }
            HStack {
                Button("취소") { model.showTransfer = false }.keyboardShortcut(.cancelAction)
                Spacer()
                Button(mustStop + (model.stopOthers ? otherCount : 0) > 0 ? "종료하고 옮기기" : "옮기기") { Task { await model.transfer() } }
                    .buttonStyle(.borderedProminent).keyboardShortcut(.defaultAction)
                    .disabled(unknownFolder && model.projectOverride.isEmpty)
            }
        }.padding(26).frame(width: 500)
    }

    func launchSheet(_ launch: Launch) -> some View {
        VStack(alignment: .leading, spacing: 14) {
            Label(launch.title, systemImage: "checkmark.circle.fill").font(.title3.bold()).foregroundStyle(.teal)
            Text(launch.detail).font(.callout).foregroundStyle(.secondary).textSelection(.enabled)
            Text(launch.command).font(.system(.caption, design: .monospaced)).textSelection(.enabled)
                .padding(8).frame(maxWidth: .infinity, alignment: .leading)
                .background(.quaternary, in: RoundedRectangle(cornerRadius: 6))
            Text("명령은 Cursor·VS Code의 터미널 탭에 붙여 넣습니다. Claude 입력창에는 넣지 마세요.").font(.caption).foregroundStyle(.secondary)
            HStack {
                Button("닫기") { model.launch = nil }.keyboardShortcut(.cancelAction)
                Spacer()
                Button("명령 복사") { model.copy(launch.command); model.launch = nil }
                ForEach(Opener.allCases.filter { $0 != .copy && $0.installed }) { opener in
                    openButton(opener, command: launch.command)
                }
            }
        }.padding(26).frame(width: 540)
    }
}

extension MainView {
    @ViewBuilder func openButton(_ opener: Opener, command: String) -> some View {
        let button = Button("\(opener.label)에서 열기") {
            model.launch = nil
            do { try model.open(command, with: opener) } catch { model.fail("\(opener.label) 열기", error) }
        }
        if opener == model.opener { button.buttonStyle(.borderedProminent) } else { button }
    }
}

/// The menu bar panel: usage at a glance without opening the window.
struct MenuContent: View {
    @ObservedObject var model: Manager
    @Environment(\.openWindow) private var openWindow
    var body: some View {
        VStack(alignment: .leading, spacing: 10) {
            HStack {
                Text("CCAM").font(.headline)
                Spacer()
                if model.busy { ProgressView().controlSize(.small) }
                Button { Task { await model.refresh() } } label: { Image(systemName: "arrow.clockwise") }
                    .buttonStyle(.borderless).disabled(model.busy).help("새로고침")
            }
            if model.accounts.isEmpty { Text("등록된 계정이 없습니다").foregroundStyle(.secondary) }
            ForEach(model.accounts) { account in
                let limits = model.usage[account.name]?.limits
                VStack(alignment: .leading, spacing: 4) {
                    HStack {
                        Text(account.name).font(.callout.bold())
                        if model.source == account.name { Text("선택됨").font(.caption2).foregroundStyle(.secondary) }
                        if model.recommended == account.name { Text("추천").font(.caption2.bold()).foregroundStyle(.teal) }
                    }
                    Meter(label: "5시간", window: limits?.five_hour)
                    Meter(label: "주간", window: limits?.weekly)
                }
                Divider()
            }
            if let at = model.lastUsageAt { Text("확인 \(relativeTime(at))").font(.caption).foregroundStyle(.secondary) }
            HStack {
                Button("창 열기") { openWindow(id: "main"); NSApp.activate(ignoringOtherApps: true) }
                Spacer()
                Button("종료") { NSApp.terminate(nil) }
            }
        }.padding(14).frame(width: 280)
    }
}
