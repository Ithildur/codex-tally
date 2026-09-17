# 定时任务

先按[Pages 文档](pages.md)手动完成一次 `sync`，再配置每 30 分钟运行的任务。替换示例中的路径，使用 Codex 所在的用户，并确保 Git 能无交互认证。电脑关机或离线时不会刷新 Pages，恢复后的下一次任务会重试。

## Linux：用户 crontab

执行 `crontab -e`，加入（路径有空格时保留引号）：

```cron
PATH=/usr/local/bin:/usr/bin:/bin
*/30 * * * * "/home/you/codex-tally/codex-tally" sync -repo "/home/you/codex-tally" -codex-home "/home/you/.codex" -log "/home/you/codex-tally/.state-codex-tally/sync.log"
```

如果 Git 或 credential helper 位于其他目录，把对应目录加入 `PATH`。使用自己的 crontab，不使用 `sudo crontab`。停用时删除该行；通过 `.state-codex-tally/sync.log` 检查执行结果。

## macOS：用户 LaunchAgent

创建 `~/Library/LaunchAgents/com.ithildur.codex-tally.sync.plist`，将所有 `/Users/you` 替换为实际用户目录；plist 中必须使用绝对路径，不能写 `~`：

```xml
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
  <key>Label</key><string>com.ithildur.codex-tally.sync</string>
  <key>ProgramArguments</key><array>
    <string>/Users/you/codex-tally/codex-tally</string>
    <string>sync</string>
    <string>-repo</string><string>/Users/you/codex-tally</string>
    <string>-codex-home</string><string>/Users/you/.codex</string>
    <string>-log</string><string>/Users/you/codex-tally/.state-codex-tally/sync.log</string>
  </array>
  <key>EnvironmentVariables</key><dict>
    <key>PATH</key><string>/opt/homebrew/bin:/usr/local/bin:/usr/bin:/bin</string>
  </dict>
  <key>StartInterval</key><integer>1800</integer>
  <key>RunAtLoad</key><true/>
</dict></plist>
```

```bash
plutil -lint ~/Library/LaunchAgents/com.ithildur.codex-tally.sync.plist
launchctl bootstrap gui/$(id -u) ~/Library/LaunchAgents/com.ithildur.codex-tally.sync.plist
# 停用
launchctl bootout gui/$(id -u)/com.ithildur.codex-tally.sync
```

这是用户登录期间运行的任务，休眠期间不保证准点执行。配置依据：[Apple LaunchAgent 文档](https://developer.apple.com/library/archive/documentation/MacOSX/Conceptual/BPSystemStartup/Chapters/CreatingLaunchdJobs.html)。

## Windows：任务计划程序

在当前用户的 PowerShell 中执行。示例仅在该用户已登录时运行，以复用其 Git Credential Manager / SSH agent；不需要把密码写入任务：

```powershell
$repo = "$env:USERPROFILE\codex-tally"
$codexDataPath = "$env:USERPROFILE\.codex"
$arguments = 'sync -repo "{0}" -codex-home "{1}" -log "{0}\.state-codex-tally\sync.log"' -f $repo, $codexDataPath
$action = New-ScheduledTaskAction -Execute "$repo\codex-tally.exe" -Argument $arguments -WorkingDirectory $repo
$trigger = New-ScheduledTaskTrigger -Once -At (Get-Date).AddMinutes(1) -RepetitionInterval (New-TimeSpan -Minutes 30)
$principal = New-ScheduledTaskPrincipal -UserId ([System.Security.Principal.WindowsIdentity]::GetCurrent().Name) -LogonType Interactive -RunLevel Limited
$settings = New-ScheduledTaskSettingsSet -MultipleInstances IgnoreNew -StartWhenAvailable -ExecutionTimeLimit (New-TimeSpan -Minutes 12)
Register-ScheduledTask -TaskName 'Codex Tally Sync' -Action $action -Trigger $trigger -Principal $principal -Settings $settings

# 查看最近执行结果与日志
Get-ScheduledTaskInfo -TaskName 'Codex Tally Sync'
Get-Content "$repo\.state-codex-tally\sync.log" -Tail 30
# 停用
Unregister-ScheduledTask -TaskName 'Codex Tally Sync' -Confirm:$false
```

确保当前用户的 `PATH` 中有 `git.exe`。使用 WSL Codex 的用户应在 WSL 内配置 Linux 任务，并确认发行版正在运行，不要用 Windows 用户目录代替 WSL 的家目录。配置依据：[Microsoft 定时触发器文档](https://learn.microsoft.com/en-us/powershell/module/scheduledtasks/new-scheduledtasktrigger)。
