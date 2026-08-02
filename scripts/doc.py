
import subprocess
import tempfile

import os

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
        tmp = tempfile.NamedTemporaryFile(delete=False)  # noqa: SIM115
        with open(md) as f:
                md_string = f.read()
                # Replace double backslash with single. GitHub requires double.
                # Pandoc doesn't for escaping the underscore in math. Could
                # potentially become a problem further down the line if I use
                # other constructs that do require double backslash.
                md_string = md_string.replace(r"\\", "\\")
                res = md_string.encode("utf-8")
                tmp.write(res)
                tmp.close()
                out = subprocess.check_output(["cat", tmp.name])
                txt = subprocess.check_output([cmd, "--mathml", "--from=markdown", "--to=html", tmp.name])
                string = txt.decode(encoding="utf-8")
        os.unlink(tmp.name)
    else:
        string = "<p>Built without Pandoc</p>"
    with open(tmpl, "r") as f:
        template = f.read()
        html = template.replace("INSERT HERE", string)

    with open(dst, "w") as f:
        f.write(html)
