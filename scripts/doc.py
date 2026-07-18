
import subprocess

files = [
    ("doc.md", "app/pandoc_partial/help_template.html", "app/tmpl/partials/help.html")
]

cmd = "pandoc"
has_pandoc = True
try:
    subprocess.check_output([cmd, "-h"])
except FileNotFoundError:
    has_pandoc = False
    print("Warning: Pandoc not installed. Skipping web documentation")

for md, tmpl, dst in files:
    if has_pandoc:
        txt = subprocess.check_output([cmd, "--from=markdown", "--to=html", md])
        string = txt.decode(encoding="utf-8")
    else:
        string = "<p>Built without Pandoc</p>"
    with open(tmpl, "r") as f:
        template = f.read()
        html = template.replace("INSERT HERE", string)

    with open(dst, "w") as f:
        f.write(html)
