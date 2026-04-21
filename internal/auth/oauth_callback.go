// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// oauth_callback.go holds the static HTML templates served by the local
// OAuth callback HTTP server defined in oauth_provider.go. The four pages
// cover: authorization success, CLI access not yet enabled for the org
// (with an in-page admin apply flow), user-level access denial, and
// channel-level denial.
package auth

const successHTML = `<!doctype html>
<html>
  <head>
    <meta charset="utf-8" />
    <title>钉钉 CLI</title>
    <style>
      body {
        font-family:
          -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto,
          "Helvetica Neue", Arial, sans-serif;
        display: flex;
        justify-content: center;
        align-items: center;
        min-height: 100vh;
        margin: 0;
        background: #f5f5f5;
        padding: 20px;
      }
      .card {
        height: 600px;
        width: 480px;
        border-radius: 16px;
        background: #ffffff;
        box-sizing: border-box;
        border: 1px solid #f2f2f6;
        box-shadow: 0px 2px 4px 0px rgba(0, 0, 0, 0.12);
        padding: 32px 24px 24px;
        text-align: center;
        display: flex;
        justify-content: center;
        align-items: center;
        flex-direction: column;
      }
      .lock-icon {
        width: 120px;
        height: 120px;
        margin: 0 auto;
        object-fit: contain;
        display: block;
      }
      h1 {
        margin: 8px 0 0;
        font-family:
          "PingFang SC",
          -apple-system,
          BlinkMacSystemFont,
          "Segoe UI",
          Roboto,
          "Helvetica Neue",
          Arial,
          sans-serif;
        font-size: 18px;
        font-weight: 600;
        line-height: 44px;
        text-align: center;
        letter-spacing: normal;
        color: #181c1f;
      }
      p {
        margin: 0;
        font-family:
          "PingFang SC",
          -apple-system,
          BlinkMacSystemFont,
          "Segoe UI",
          Roboto,
          "Helvetica Neue",
          Arial,
          sans-serif;
        font-size: 14px;
        font-weight: normal;
        line-height: 21px;
        text-align: center;
        letter-spacing: normal;
        color: rgba(24, 28, 31, 0.6);
      }
    </style>
  </head>
  <body>
    <div class="card">
      <img
        class="lock-icon"
        src="https://img.alicdn.com/imgextra/i4/O1CN01fS3xxz1vbzZSGjbe0_!!6000000006192-2-tps-480-480.png"
        alt="lock icon"
      />
      <h1>授权成功</h1>
      <p>请返回终端继续操作。此页面可以关闭。</p>
    </div>
  </body>
</html>`

const notEnabledHTML = `<!doctype html>
<html lang="zh-CN">
  <head>
    <meta charset="utf-8" />
    <meta name="viewport" content="width=device-width, initial-scale=1" />
    <title>钉钉 CLI</title>
    <style>
      * {
        margin: 0;
        padding: 0;
        box-sizing: border-box;
      }
      body {
        font-family:
          -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto,
          "Helvetica Neue", Arial, sans-serif;
        display: flex;
        justify-content: center;
        align-items: center;
        min-height: 100vh;
        background: #fff;
        padding: 20px;
      }
      .container {
        text-align: center;
        width: 100%;
        max-width: 400px;
        border-radius: 16px;
        background: #ffffff;
        border: 1px solid #f2f2f6;
        box-shadow: 0 2px 4px 0 rgba(0, 0, 0, 0.12);
        padding: 32px 24px 24px;
      }
      .lock-icon {
        width: 120px;
        height: 120px;
        margin: 0 auto;
        object-fit: contain;
        display: block;
      }
      h1 {
        font-family:
          "PingFang SC",
          -apple-system,
          BlinkMacSystemFont,
          "Segoe UI",
          Roboto,
          "Helvetica Neue",
          Arial,
          sans-serif;
        font-size: 18px;
        font-weight: 600;
        line-height: 44px;
        text-align: center;
        color: #181c1f;
      }
      p {
        font-family:
          "PingFang SC",
          -apple-system,
          BlinkMacSystemFont,
          "Segoe UI",
          Roboto,
          "Helvetica Neue",
          Arial,
          sans-serif;
        font-size: 14px;
        font-weight: 400;
        line-height: 21px;
        text-align: center;
        color: rgba(24, 28, 31, 0.6);
        margin-bottom: 24px;
      }
      .form-group {
        text-align: left;
        margin-bottom: 24px;
      }
      .form-label {
        font-family:
          "PingFang SC",
          -apple-system,
          BlinkMacSystemFont,
          "Segoe UI",
          Roboto,
          "Helvetica Neue",
          Arial,
          sans-serif;
        font-size: 14px;
        font-weight: 400;
        line-height: 14px;
        color: rgba(24, 28, 31, 0.6);
        margin-top: 38px;
        margin-bottom: 8px;
        display: block;
      }
      .select-wrapper {
        position: relative;
      }
      .custom-select-trigger {
        width: 100%;
        height: 40px;
        display: flex;
        align-items: center;
        justify-content: space-between;
        padding: 0 16px;
        gap: 12px;
        border-radius: 8px;
        background: #ffffff;
        border: 1px solid rgba(126, 134, 142, 0.24);
        cursor: pointer;
        color: #181c1f;
        text-align: left;
      }
      .custom-select-text {
        flex: 1;
        font-size: 14px;
        line-height: 24px;
        color: rgba(24, 28, 31, 0.48);
      }
      .custom-select.has-value .custom-select-text {
        color: #181c1f;
      }
      .custom-select-arrow {
        width: 16px;
        height: 16px;
        flex-shrink: 0;
        background-image: url("https://img.alicdn.com/imgextra/i1/O1CN01MzGSB21oZ3iyQ8H5e_!!6000000005238-55-tps-16-16.svg");
        background-repeat: no-repeat;
        background-size: 16px 16px;
        background-position: center;
        opacity: 0.5;
      }
      .custom-select-menu {
        position: absolute;
        left: 0;
        right: 0;
        top: calc(100% + 8px);
        background: #ffffff;
        border-radius: 10px;
        padding: 8px 12px;
        list-style: none;
        margin: 0;
        box-shadow: 0 6px 18px rgba(0, 0, 0, 0.12);
        display: none;
        z-index: 20;
      }
      .custom-select.open .custom-select-menu {
        display: block;
      }
      .custom-select-option {
        width: 100%;
        height: 40px;
        border: none;
        background: transparent;
        text-align: left;
        padding: 8px 12px;
        border-radius: 8px;
        font-size: 14px;
        line-height: 24px;
        color: #181c1f;
        cursor: pointer;
      }
      .custom-select-option:hover {
        background: rgba(126, 134, 142, 0.16);
      }
      .custom-select-option.is-active {
        background: #e8eaee;
      }
      .btn {
        width: 100%;
        height: 40px;
        border-radius: 1000px;
        background: #0066ff;
        border: none;
        cursor: pointer;
        font-family:
          "PingFang SC",
          -apple-system,
          BlinkMacSystemFont,
          "Segoe UI",
          Roboto,
          "Helvetica Neue",
          Arial,
          sans-serif;
        font-size: 16px;
        font-weight: 500;
        line-height: 24px;
        color: #ffffff;
      }
      .btn:hover {
        background: #0b6cff;
      }
      .btn:disabled {
        background: #d9d9d9;
        cursor: not-allowed;
      }
      .link {
        color: #1890ff;
        font-size: 14px;
        text-decoration: none;
        margin-top: 16px;
        display: inline-block;
      }
      .success-msg {
        display: none;
        width: 100%;
        height: 60px;
        gap: 12px;
        padding: 16px 20px;
        margin-top: 50px;
        margin-bottom: 16px;
        background: #eaf1ff;
        border-radius: 12px;
        align-items: center;
      }
      .success-msg-icon {
        width: 24px;
        height: 24px;
        flex-shrink: 0;
      }
      .success-msg-text {
        flex: 1;
        font-size: 14px;
        line-height: 22px;
        color: #181c1f;
      }
      .error-msg {
        color: #ff4d4f;
        font-size: 14px;
        margin-top: 8px;
        display: none;
      }
      .loading {
        display: inline-block;
        width: 16px;
        height: 16px;
        border: 2px solid #fff;
        border-top-color: transparent;
        border-radius: 50%;
        animation: spin 0.8s linear infinite;
        margin-right: 8px;
        vertical-align: middle;
      }
      @keyframes spin {
        to {
          transform: rotate(360deg);
        }
      }
    </style>
  </head>
  <body>
    <div class="container">
      <img
        class="lock-icon"
        src="https://img.alicdn.com/imgextra/i4/O1CN01fS3xxz1vbzZSGjbe0_!!6000000006192-2-tps-480-480.png"
        alt="lock icon"
      />
      <h1>该组织尚未开启 CLI 数据访问权限</h1>
      <p>
        你所选择的组织管理员尚未开启<br />「允许成员通过 CLI
        访问其个人数据」的权限。
      </p>

      <div class="form-group">
        <label class="form-label">选择一位主管理员发送开通申请</label>
        <div class="select-wrapper">
          <div class="custom-select" id="adminSelect">
            <button
              type="button"
              class="custom-select-trigger"
              aria-haspopup="listbox"
              aria-expanded="false"
            >
              <span class="custom-select-text">加载中...</span>
              <span class="custom-select-arrow"></span>
            </button>
            <ul class="custom-select-menu" role="listbox" id="adminMenu"></ul>
            <input type="hidden" name="adminStaffId" value="" />
          </div>
        </div>
        <div id="errorMsg" class="error-msg"></div>
      </div>

      <div id="successMsg" class="success-msg">
        <svg
          class="success-msg-icon"
          viewBox="0 0 16 16"
          fill="none"
          xmlns="http://www.w3.org/2000/svg"
        >
          <path
            d="M8 1.33333C4.32 1.33333 1.33333 4.32 1.33333 8C1.33333 11.68 4.32 14.6667 8 14.6667C11.68 14.6667 14.6667 11.68 14.6667 8C14.6667 4.32 11.68 1.33333 8 1.33333ZM8 13.3333C5.05333 13.3333 2.66667 10.9467 2.66667 8C2.66667 5.05333 5.05333 2.66667 8 2.66667C10.9467 2.66667 13.3333 5.05333 13.3333 8C13.3333 10.9467 10.9467 13.3333 8 13.3333ZM7.33333 9.33333H8.66667V10.6667H7.33333V9.33333ZM7.33333 5.33333H8.66667V8H7.33333V5.33333Z"
            fill="#0066FF"
          />
        </svg>
        <span class="success-msg-text"
          >已向管理员发送权限申请，正在等待审核<br />审核通过后，请返回终端继续操作</span
        >
      </div>

      <button id="applyBtn" class="btn" disabled>立即申请</button>
      <a id="backLink" class="link" href="#">返回选择其他组织</a>
    </div>

    <script>
      const adminSelect = document.getElementById("adminSelect");
      const trigger = adminSelect.querySelector(".custom-select-trigger");
      const text = adminSelect.querySelector(".custom-select-text");
      const menu = document.getElementById("adminMenu");
      const hiddenInput = adminSelect.querySelector('input[name="adminStaffId"]');
      const btn = document.getElementById("applyBtn");
      const successMsg = document.getElementById("successMsg");
      const errorMsg = document.getElementById("errorMsg");
      const backLink = document.getElementById("backLink");

      let admins = [];
      let clientId = "";
      let applySent = false;
      let selectedAdminId = "";
      let pollTimer = null;

      function closeMenu() {
        adminSelect.classList.remove("open");
        trigger.setAttribute("aria-expanded", "false");
      }

      function openMenu() {
        adminSelect.classList.add("open");
        trigger.setAttribute("aria-expanded", "true");
      }

      function showError(msg) {
        errorMsg.textContent = msg;
        errorMsg.style.display = "block";
      }

      function hideError() {
        errorMsg.style.display = "none";
      }

      function setSelected(staffId, name) {
        hiddenInput.value = staffId || "";
        text.textContent = name || "请选择";
        if (staffId) {
          adminSelect.classList.add("has-value");
        } else {
          adminSelect.classList.remove("has-value");
        }
        btn.disabled = applySent || !staffId;
      }

      function renderAdminOptions(list) {
        menu.innerHTML = "";
        list.forEach(function (admin) {
          const li = document.createElement("li");
          const option = document.createElement("button");
          option.type = "button";
          option.className = "custom-select-option";
          option.setAttribute("data-value", admin.staffId);
          option.textContent = admin.name;

          if (selectedAdminId && admin.staffId === selectedAdminId) {
            option.classList.add("is-active");
          }

          option.addEventListener("click", function () {
            selectedAdminId = admin.staffId;
            const all = menu.querySelectorAll(".custom-select-option");
            all.forEach(function (item) {
              item.classList.remove("is-active");
            });
            option.classList.add("is-active");
            setSelected(admin.staffId, admin.name);
            closeMenu();
            hideError();
          });

          li.appendChild(option);
          menu.appendChild(li);
        });
      }

      function setAppliedState() {
        btn.disabled = true;
        btn.textContent = "立即申请";
        trigger.disabled = true;
        adminSelect.classList.remove("open");
        successMsg.style.display = "flex";
        backLink.style.pointerEvents = "none";
        backLink.style.color = "#999";
        backLink.onclick = function (e) {
          e.preventDefault();
          return false;
        };
        startPolling();
      }

      function startPolling() {
        if (pollTimer) return;
        pollTimer = setInterval(checkAuthStatus, 5000);
        checkAuthStatus();
      }

      function stopPolling() {
        if (pollTimer) {
          clearInterval(pollTimer);
          pollTimer = null;
        }
      }

      async function checkAuthStatus() {
        try {
          const res = await fetch("/api/cliAuthEnabled");
          const data = await res.json();
          if (data.success && data.result && data.result.cliAuthEnabled) {
            stopPolling();
            location.href = "/success";
          }
        } catch (e) {
          console.error("Poll error", e);
        }
      }

      async function loadAdmins() {
        try {
          const res = await fetch("/api/superAdmin");
          const data = await res.json();
          if (data.success && data.result && data.result.length > 0) {
            admins = data.result;
            renderAdminOptions(admins);

            if (selectedAdminId) {
              const selected = admins.find(function (a) {
                return a.staffId === selectedAdminId;
              });
              if (selected) {
                setSelected(selected.staffId, selected.name);
              } else {
                setSelected("", "请选择");
              }
            } else {
              setSelected("", "请选择");
            }
          } else {
            setSelected("", "暂无可选管理员");
            trigger.disabled = true;
            showError((data && data.errorMsg) || "获取管理员列表失败");
          }
        } catch (e) {
          setSelected("", "加载失败");
          trigger.disabled = true;
          showError("网络错误，请重试");
        }
      }

      async function init() {
        try {
          const statusRes = await fetch("/api/status");
          const status = await statusRes.json();
          clientId = status.clientId || "";
          applySent = status.applySent || false;
          selectedAdminId = status.selectedAdminId || "";

          if (clientId) {
            const port = location.port;
            const redirectUri = encodeURIComponent(
              "http://127.0.0.1:" + port + "/callback"
            );
            backLink.href =
              "https://login.dingtalk.com/oauth2/auth?client_id=" +
              clientId +
              "&prompt=consent&redirect_uri=" +
              redirectUri +
              "&response_type=code&scope=openid+corpid";
          }

          if (applySent) {
            setAppliedState();
          }
        } catch (e) {
          console.error("Failed to load status", e);
        }

        await loadAdmins();
      }

      trigger.addEventListener("click", function () {
        if (trigger.disabled) return;
        if (adminSelect.classList.contains("open")) {
          closeMenu();
        } else {
          openMenu();
        }
      });

      document.addEventListener("click", function (event) {
        if (!adminSelect.contains(event.target)) {
          closeMenu();
        }
      });

      btn.onclick = async function () {
        const value = hiddenInput.value;
        if (!value) return;

        btn.disabled = true;
        btn.innerHTML = '<span class="loading"></span>申请中...';
        hideError();
        try {
          const res = await fetch(
            "/api/sendApply?adminStaffId=" + encodeURIComponent(value)
          );
          const data = await res.json();
          if (data.success && data.result) {
            setAppliedState();
          } else {
            showError(data.errorMsg || "申请失败，请重试");
            btn.disabled = false;
            btn.textContent = "立即申请";
          }
        } catch (e) {
          showError("网络错误，请重试");
          btn.disabled = false;
          btn.textContent = "立即申请";
        }
      };

      init();
    </script>
  </body>
</html>`

const accessDeniedHTML = `<!doctype html>
<html lang="zh-CN">
  <head>
    <meta charset="utf-8" />
    <meta name="viewport" content="width=device-width, initial-scale=1" />
    <title>钉钉 CLI</title>
    <style>
      body {
        font-family:
          -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto,
          "Helvetica Neue", Arial, sans-serif;
        display: flex;
        justify-content: center;
        align-items: center;
        min-height: 100vh;
        margin: 0;
        background: #f5f5f5;
        padding: 20px;
      }
      .card {
        height: 600px;
        width: 480px;
        border-radius: 16px;
        background: #ffffff;
        box-sizing: border-box;
        border: 1px solid #f2f2f6;
        box-shadow: 0px 2px 4px 0px rgba(0, 0, 0, 0.12);
        padding: 32px 24px 24px;
        text-align: center;
        display: flex;
        justify-content: center;
        align-items: center;
        flex-direction: column;
      }
      .lock-icon {
        width: 120px;
        height: 120px;
        margin: 0 auto;
        object-fit: contain;
        display: block;
      }
      h1 {
        margin: 8px 0 0;
        font-family:
          "PingFang SC",
          -apple-system,
          BlinkMacSystemFont,
          "Segoe UI",
          Roboto,
          "Helvetica Neue",
          Arial,
          sans-serif;
        font-size: 18px;
        font-weight: 600;
        line-height: 44px;
        text-align: center;
        letter-spacing: normal;
        color: #181c1f;
      }
      p {
        margin: 0;
        font-family:
          "PingFang SC",
          -apple-system,
          BlinkMacSystemFont,
          "Segoe UI",
          Roboto,
          "Helvetica Neue",
          Arial,
          sans-serif;
        font-size: 14px;
        font-weight: normal;
        line-height: 21px;
        text-align: center;
        letter-spacing: normal;
        color: rgba(24, 28, 31, 0.6);
      }
    </style>
  </head>
  <body>
    <div class="card">
      <img
        class="lock-icon"
        src="https://img.alicdn.com/imgextra/i4/O1CN01fS3xxz1vbzZSGjbe0_!!6000000006192-2-tps-480-480.png"
        alt="lock icon"
      />
      <h1>无权限访问</h1>
      <p>您不在该组织的 CLI 授权人员范围内。请联系组织管理员将您加入授权名单。此页面可以关闭。</p>
    </div>
  </body>
</html>`

const channelDeniedHTML = `<!doctype html>
<html lang="zh-CN">
  <head>
    <meta charset="utf-8" />
    <meta name="viewport" content="width=device-width, initial-scale=1" />
    <title>钉钉 CLI</title>
    <style>
      body {
        font-family:
          -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto,
          "Helvetica Neue", Arial, sans-serif;
        display: flex;
        justify-content: center;
        align-items: center;
        min-height: 100vh;
        margin: 0;
        background: #f5f5f5;
        padding: 20px;
      }
      .card {
        height: 600px;
        width: 480px;
        border-radius: 16px;
        background: #ffffff;
        box-sizing: border-box;
        border: 1px solid #f2f2f6;
        box-shadow: 0px 2px 4px 0px rgba(0, 0, 0, 0.12);
        padding: 32px 24px 24px;
        text-align: center;
        display: flex;
        justify-content: center;
        align-items: center;
        flex-direction: column;
      }
      .lock-icon {
        width: 120px;
        height: 120px;
        margin: 0 auto;
        object-fit: contain;
        display: block;
      }
      h1 {
        margin: 8px 0 0;
        font-family:
          "PingFang SC",
          -apple-system,
          BlinkMacSystemFont,
          "Segoe UI",
          Roboto,
          "Helvetica Neue",
          Arial,
          sans-serif;
        font-size: 18px;
        font-weight: 600;
        line-height: 44px;
        text-align: center;
        letter-spacing: normal;
        color: #181c1f;
      }
      p {
        margin: 0;
        font-family:
          "PingFang SC",
          -apple-system,
          BlinkMacSystemFont,
          "Segoe UI",
          Roboto,
          "Helvetica Neue",
          Arial,
          sans-serif;
        font-size: 14px;
        font-weight: normal;
        line-height: 21px;
        text-align: center;
        letter-spacing: normal;
        color: rgba(24, 28, 31, 0.6);
      }
    </style>
  </head>
  <body>
    <div class="card">
      <img
        class="lock-icon"
        src="https://img.alicdn.com/imgextra/i4/O1CN01fS3xxz1vbzZSGjbe0_!!6000000006192-2-tps-480-480.png"
        alt="lock icon"
      />
      <h1>渠道未授权</h1>
      <p>当前渠道未获得该组织授权，或组织已开启渠道管控。请联系组织管理员开通渠道访问权限，或升级到最新版本的 CLI。此页面可以关闭。</p>
    </div>
  </body>
</html>`
