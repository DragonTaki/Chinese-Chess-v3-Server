# 實作狀態

以實際程式碼為準（不是 `README.md` 的功能敘述）。
狀態標記：**已完成** / **部分完成** / **僅有骨架** / **未實作**。

## 核心功能

- **身分驗證** — 已完成。雙階段交握（版本檢查 → 帳密檢查），透過
  `golang-jwt/jwt/v5` 簽發 JWT，密碼以 bcrypt 雜湊。跟 client 端的
  `AuthManager` 流程對得上。
- **聊天** — 已完成。`PacketTypeChat` → `Server.Broadcast`。
- **房間／對局**（`JoinRoom`、`LeaveRoom`、`StartGame`、`EndGame`、
  `GameAction`、`TimerSync`）— 完全未實作。`Client.Listen()` 的主迴圈只會
  處理 `PacketTypeChat`；`packettype.go` 裡定義的其他封包類型都收得到，
  但沒有任何處理邏輯。伺服器端房間／對局的邏輯是 0 行。
- **使用者註冊** — 未實作。`server/db/repository.go` 裡完全沒有
  `CreateUser` 或任何註冊用的函式。目前唯一能建立 `User` 資料的方式是
  獨立的 `test/create_fake_user.go` 腳本——還沒有真正的註冊流程。
- **心跳探測** — 已完成，跟 client 端的心跳／逾時邏輯對得上。
- **封包協定** — client（`PacketType.cs`）跟 server（`packettype.go`）的
  列舉是完全同步的（同樣 14 個值、同樣的命名）。

## 死碼／半成品檔案

- `server/broadcast.go` — 空檔案（只有 package 宣告）。真正的 `Broadcast`
  函式其實寫在 `server.go` 裡，檔名會誤導人。
- `auth.go` 裡的 `AuthMessage` struct — 定義了但沒用到，實際的解析是走
  `Packet.ParseAuthData` / `AuthData`。

## 除錯殘留（後續開發前必須先清掉）

- `server/auth.go` — `fmt.Println("Input password:", ad.Password)`，
  會把明文密碼印到 stdout。
- `server/db/repository.go` — `fmt.Println("Stored hash:", user.PasswordHash)`，
  會把 bcrypt hash 印到 stdout。

## 其他

- `main.go` 寫死監聽 `127.0.0.1:8080`——只能本機連，無法設定。
- repo 根目錄多了一份重複的 `chess.db`，跟真正在用的
  `server/db/chess.db` 並存，很可能是某次用錯工作目錄執行伺服器時產生的。
