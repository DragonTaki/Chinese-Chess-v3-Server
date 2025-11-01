[![Go](https://img.shields.io/badge/Built%20with-Go-%2300ADD8.svg?style=flat-square&logo=go&logoColor=white)](https://go.dev/)

---

## 💡 About This Project

This project is only for private use.  
Features:

- 為 [Chinese-Chess-v3](https://github.com/DragonTaki/Chinese-Chess-v3) 的配套中央伺服器系統

---

## ⚙️ Project Features 專案特點

本專案以 Golang 編寫，作為 Chinese-Chess-v3 的中央伺服器系統。  
本程式具備以下主要特點：

1. 資料庫系統

    - 目前採用 SQLite 做為系統資料庫

2. 安全性

    - 使用 JWT 方式建立連線

    - 由環境變數設定加密金鑰，而非寫死於 Code 中

    - 密碼以 Hash 過的數值儲存

3. 伺服器功能

    - TCP 連線

    - 雙重階段驗證：版本驗證 -> 身分驗證

    - 非對稱心跳探測

        - 伺服端採用廣播方式發布心跳封包

        - 每隔固定時間檢查是否收到用戶端心跳

4. 使用者系統

    - 以現今遊戲常用的 9 位數字 UID 作為玩家唯一標示符，不連號

    - Email 與密碼作為登入驗證資訊

---

## 🛡️ About Chinese Chess

Chinese chess is a strategy board game for two players.  
It is the most popular board game in East Asia.

---

## 📜 Third-Party Licenses

- 僅供個人學術研究使用，可相互探討，禁止抄襲（包含部分抄襲）
- 禁止搬運、複製或儲存於其他儲存庫中
- 禁止當作作業 Project
- 禁止當作作業 Project
- 禁止當作作業 Project

> This repository **does not provide any license or redistribution rights** for its own code.  
> It is intended for **private use only**.

---

## 👤 About Author

- [Discord](https://discord.gg/GDMSyVt)
- [Twitch](https://bit.ly/DragonTakiTwitch)
- [YouTube](https://bit.ly/DragonTakiYTNew)
- [Twitter](https://twitter.com/MacroDragonTaki)
- [Fur Affinity](https://bit.ly/DragonTakiFA)
- [巴哈姆特](https://bit.ly/DragonTakiBaha)
