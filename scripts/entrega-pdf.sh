#!/usr/bin/env bash
# Gera o PDF de entrega a partir do Markdown, com os diagramas Mermaid renderizados, usando o
# Chrome em modo headless (marked e mermaid carregados do jsDelivr na hora da impressão).
#
#   scripts/entrega-pdf.sh [docs/DOCUMENTO_ENTREGA_FASE4.md] [saída.pdf]
set -euo pipefail

SRC=${1:-docs/DOCUMENTO_ENTREGA_FASE4.md}
OUT=${2:-${SRC%.md}.pdf}
CHROME=${CHROME:-"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome"}
[ -x "$CHROME" ] || CHROME=$(command -v google-chrome || command -v chromium || true)
[ -n "$CHROME" ] || { echo "Chrome não encontrado (defina CHROME)" >&2; exit 1; }
grep -q '</script' "$SRC" && { echo "o Markdown não pode conter </script>" >&2; exit 1; }

TMP=$(mktemp -d)
trap 'rm -rf "$TMP"' EXIT
HTML="$TMP/entrega.html"

SRCDIR=$(cd "$(dirname "$SRC")" && pwd)
{
  printf '<!doctype html>\n<html lang="pt-BR"><head><meta charset="utf-8"><base href="file://%s/">\n' "$SRCDIR"
  cat <<'HEAD'
<title>Tech Challenge Fase 4</title>
<style>
  @page { size: A4; margin: 16mm 14mm; }
  body { font: 10.5pt/1.5 -apple-system, "Segoe UI", Helvetica, Arial, sans-serif; color: #1f2328; }
  h1 { font-size: 20pt; margin: 0 0 4pt; } h2 { font-size: 14pt; margin: 18pt 0 6pt; border-bottom: 1px solid #d0d7de; padding-bottom: 3pt; }
  h3 { font-size: 11.5pt; margin: 12pt 0 4pt; }
  table { border-collapse: collapse; width: 100%; margin: 6pt 0; font-size: 9.5pt; page-break-inside: avoid; }
  th, td { border: 1px solid #d0d7de; padding: 4pt 6pt; vertical-align: top; text-align: left; }
  th { background: #f6f8fa; }
  code { font: 9pt ui-monospace, Menlo, monospace; background: #f6f8fa; padding: 0 3pt; border-radius: 3px; }
  blockquote { margin: 6pt 0; padding: 4pt 10pt; color: #57606a; border-left: 3px solid #d0d7de; }
  a { color: #0969da; text-decoration: none; }
  pre.mermaid { text-align: center; background: none; page-break-inside: avoid; }
  pre.mermaid svg { max-width: 100%; height: auto; }
  img { max-width: 100%; }
  ul { padding-left: 16pt; } li { margin: 1pt 0; }
</style></head><body>
<main id="out"></main>
<script type="text/markdown" id="md">
HEAD
  cat "$SRC"
  cat <<'TAIL'
</script>
<script src="https://cdn.jsdelivr.net/npm/marked@12.0.2/marked.min.js"></script>
<script src="https://cdn.jsdelivr.net/npm/mermaid@10.9.1/dist/mermaid.min.js"></script>
<script>
  const out = document.getElementById('out');
  out.innerHTML = marked.parse(document.getElementById('md').textContent);
  out.querySelectorAll('pre > code.language-mermaid').forEach(code => {
    const pre = document.createElement('pre');
    pre.className = 'mermaid';
    pre.textContent = code.textContent;
    code.parentElement.replaceWith(pre);
  });
  mermaid.initialize({ startOnLoad: false, theme: 'neutral', securityLevel: 'loose', flowchart: { htmlLabels: true } });
  mermaid.run();
</script>
</body></html>
TAIL
} > "$HTML"
[ -n "${KEEP_HTML:-}" ] && cp "$HTML" "$KEEP_HTML"

case $OUT in /*) ABS=$OUT ;; *) ABS=$PWD/$OUT ;; esac
"$CHROME" --headless=new --disable-gpu --no-pdf-header-footer --virtual-time-budget=20000 \
  --run-all-compositor-stages-before-draw --print-to-pdf="$ABS" "file://$HTML" 2>/dev/null
echo "PDF gerado: $OUT"
