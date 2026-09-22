# 修改規劃

依據 `STATUS.md` 目前的缺口排出的優先順序。

1. **先清掉洩漏的除錯 log。** `auth.go` 每次驗證都會把明文密碼印到
   stdout，`repository.go` 會印出 bcrypt hash。這是正在發生的憑證洩漏，
   優先度最高，先修。
2. **實作房間／對局的封包處理**（`JoinRoom`、`LeaveRoom`、`StartGame`、
   `EndGame`、`GameAction`、`TimerSync`）。這才是真正在跑對局的邏輯，
   目前完全不存在——`Client.Listen()` 現在只處理聊天。
3. **做一個真正的使用者註冊流程**——在 `repository.go` 裡加
   `CreateUser`，再配一個註冊用的封包類型。目前唯一能建立使用者的方式是
   獨立的 `test/create_fake_user.go` 腳本。
4. **清掉 repo 根目錄多餘的 `chess.db`**，並找出是哪次用錯工作目錄執行
   造成的、順便修掉。只留 `server/db/chess.db` 這一份。
5. **補上或移除 `server/broadcast.go`。** 目前是個檔名有誤導性的空檔案；
   要嘛把 `Server.Broadcast` 搬進去，要嘛就刪掉這個檔案。
6. **移除 `auth.go` 裡沒用到的 `AuthMessage` struct。**
7. **讓監聽位址可設定**（環境變數或設定檔），不要再寫死
   `127.0.0.1:8080`——不這樣做的話，就只能拿來做 localhost 測試。

## 跟 client 端對齊

這份規劃要搭配
[`Chinese-Chess-v3/docs/PLAN.md`](https://github.com/DragonTaki/Chinese-Chess-v3/blob/main/docs/PLAN.md)
一起看：client 端的連線層目前做出來了但從沒被實際建立過實例，而這邊完全
沒有房間／對局的處理邏輯。這兩邊單獨繼續往下做都沒意義——這裡的第 2 項
跟 client 端規劃的第 1 項其實是同一個功能，兩邊都還沒做完。
