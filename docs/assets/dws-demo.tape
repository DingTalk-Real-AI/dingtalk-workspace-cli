# dws demo GIF — reproducible via VHS (https://github.com/charmbracelet/vhs)
#   vhs docs/assets/dws-demo.tape
# No PII / no side effects: only --dry-run, which previews the request without executing.

Output docs/assets/dws-demo.gif

Set Shell bash
Set FontSize 20
Set Width 1280
Set Height 460
Set Padding 25
Set Margin 16
Set MarginFill "#0d0e14"
Set BorderRadius 10
Set WindowBar Colorful
Set Theme "TokyoNight"
Set TypingSpeed 32ms

# Themed prompt, set off-camera
Hide
Type "export PS1='\[\e[32m\]dws \[\e[36m\]❯\[\e[0m\] '"
Enter
Type "clear"
Enter
Show

Sleep 400ms

# 1) Resolve a person — the exact tool call is shown transparently, nothing executed
Type "dws contact user search --query 'Wei Zhang' --dry-run --jq '{tool:.invocation.tool, args:.invocation.params}'"
Sleep 500ms
Enter
Sleep 1700ms

Type "clear"
Enter
Sleep 400ms

# 2) Find who owns an area
Type "dws aisearch person --query 'who owns payments' --dry-run --jq '{tool:.invocation.tool, args:.invocation.params}'"
Sleep 500ms
Enter
Sleep 1700ms

Type "clear"
Enter
Sleep 400ms

# 3) The pitch — numbers match the README
Type "echo '18 products · 330 commands · structured JSON for AI agents · zero-trust'"
Sleep 500ms
Enter
Sleep 2300ms
