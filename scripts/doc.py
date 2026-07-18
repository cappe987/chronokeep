
import subprocess

files = [
    ("doc.md", "app/pandoc_partial/help_template.html", "app/tmpl/partials/help.html")
]

for md, tmpl, dst in files:
    txt = subprocess.check_output(["pandoc", "--from=markdown", "--to=html", md])
    with open(tmpl, "r") as f:
        template = f.read()
        string = txt.decode(encoding="utf-8")
        html = template.replace("INSERT HERE", string)

    with open(dst, "w") as f:
        f.write(html)
