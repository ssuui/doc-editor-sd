# Replace all zh-cn/ internal links in md files
# Rules:
# - (zh-cn/xxx.md) -> (xxx)  (remove zh-cn/ prefix and .md suffix)
# - (zh-cn/xxx.md#anchor) -> (xxx#anchor)
# - (zh-cn/xxx?id=...) -> (xxx?id=...)
# - (zh-cn/xxx) -> (xxx)  (no .md suffix to remove)
# - Keep image links (zh-cn/imgs/xxx.png) unchanged
# - Keep external links (http/https) unchanged

$mdDir = "D:\Downloads\zh-cn"
$files = Get-ChildItem -Path $mdDir -Recurse -Filter "*.md"

$pattern = '\(zh-cn/((?!imgs/).+?\.md(#.*?)?)\)'

$totalReplacements = 0
$fileCount = 0

foreach ($file in $files) {
    $content = Get-Content -Path $file.FullName -Raw -Encoding UTF8
    if ($null -eq $content) { continue }

    $newContent = $content -replace $pattern, {
        param($match)
        $innerPath = $match.Groups[1].Value
        # Remove .md extension (but keep #anchor and ?query parts)
        $newPath = $innerPath -replace '\.md$', ''
        return "($newPath)"
    }

    # Also handle (zh-cn/xxx) without .md suffix but not images
    $pattern2 = '\(zh-cn/((?!imgs/)[^)]+?)(?<!\.png)(?<!\.jpg)(?<!\.jpeg)(?<!\.gif)(?<!\.webp)(?<!\.svg)\)'
    # This is tricky - let's use a different approach
    # Handle links like (zh-cn/annotation) or (zh-cn/annotation#xxx) or (zh-cn/awesome-components?id=xxx)
    $pattern2 = '\(zh-cn/((?!imgs/)[^)\s]+\.(md|png|jpg|jpeg|gif|webp|svg))\)'
    # Already handled above for .md, now handle non-.md non-image links
    # Actually let me just handle remaining zh-cn/ links that aren't images

    $pattern3 = '\(zh-cn/((?!imgs/)[^)\s]+?[^)]*?)\)'
    # But avoid matching .md ones again (already handled), and avoid images

    # Better approach: just do a single pass
    # Reset and do it properly
    $content = Get-Content -Path $file.FullName -Raw -Encoding UTF8
    $newContent = $content

    # Match all (zh-cn/xxx) links that are NOT images (no .png/.jpg/.gif/.webp/.svg extension)
    $regex = '\(zh-cn/((?!imgs/)[^)\s]+?)\)'
    $matches = [regex]::Matches($content, $regex)
    $replacements = @{}

    foreach ($m in $matches) {
        $fullMatch = $m.Value
        $innerPath = $m.Groups[1].Value

        # Skip if already processed
        if ($replacements.ContainsKey($fullMatch)) { continue }

        # Skip image extensions
        if ($innerPath -match '\.(png|jpg|jpeg|gif|webp|svg)$') { continue }

        # Remove .md suffix if present
        $newPath = $innerPath -replace '\.md$', ''

        $newLink = "($newPath)"
        $replacements[$fullMatch] = $newLink
    }

    if ($replacements.Count -gt 0) {
        foreach ($old in $replacements.Keys) {
            $newContent = $newContent.Replace($old, $replacements[$old])
        }
        $fileCount++
        $totalReplacements += $replacements.Count
        Write-Host "Updated $($file.Name): $($replacements.Count) replacements"
        Set-Content -Path $file.FullName -Value $newContent -NoNewline -Encoding UTF8
    }
}

Write-Host "`nDone! Total: $fileCount files updated, $totalReplacements replacements made."
