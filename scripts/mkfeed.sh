#!/bin/sh
# Строит opkg-репозиторий (Packages, Packages.gz, index.html) из out/pkg/*.ipk
# в каталоги out/_pages/<arch>/ — для публикации на GitHub Pages.
set -eu
cd "$(dirname "$0")/.."
PAGES="out/_pages"
rm -rf "$PAGES"
mkdir -p "$PAGES"

for ipk in out/pkg/*.ipk; do
    [ -f "$ipk" ] || continue
    tmp="$(mktemp -d)"
    tar -xzf "$ipk" -C "$tmp" ./control.tar.gz
    mkdir "$tmp/c"
    tar -xzf "$tmp/control.tar.gz" -C "$tmp/c" ./control
    arch="$(sed -n 's/^Architecture: //p' "$tmp/c/control")"
    dir="$PAGES/$arch"
    mkdir -p "$dir"
    cp "$ipk" "$dir/"
    base="$(basename "$ipk")"
    {
        sed '/^$/d' "$tmp/c/control"
        echo "Filename: $base"
        echo "Size: $(wc -c < "$ipk" | tr -d ' ')"
        echo "SHA256sum: $(sha256sum "$ipk" | cut -d' ' -f1)"
        echo
    } >> "$dir/Packages"
    rm -rf "$tmp"
done

for dir in "$PAGES"/*/; do
    [ -d "$dir" ] || continue
    gzip -9 -k -f "${dir}Packages"
    {
        echo '<html><head><title>usque-gui repository</title></head><body><pre>'
        for f in "$dir"*; do b=$(basename "$f"); [ "$b" = index.html ] || echo "<a href=\"$b\">$b</a>"; done
        echo '</pre></body></html>'
    } > "${dir}index.html"
done

{
    echo '<html><head><title>usque-gui repository</title></head><body><h1>usque-gui opkg repository</h1><pre>'
    for d in "$PAGES"/*/; do n=$(basename "$d"); echo "<a href=\"$n/\">$n/</a>"; done
    echo '</pre></body></html>'
} > "$PAGES/index.html"
echo "репозиторий: $PAGES"
