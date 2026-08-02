
import subprocess

files = [
    ("doc/help.md", "app/pandoc_partial/help_template.html", "app/tmpl/help.html"),
    ("doc/examples.md", "app/pandoc_partial/examples_template.html", "app/tmpl/examples.html"),
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
        txt = subprocess.check_output([cmd, "--mathml", "--from=markdown", "--to=html", md])
        string = txt.decode(encoding="utf-8")
    else:
        string = "<p>Built without Pandoc</p>"
    with open(tmpl, "r") as f:
        template = f.read()
        html = template.replace("INSERT HERE", string)

    with open(dst, "w") as f:
        f.write(html)
