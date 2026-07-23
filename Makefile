
.PHONY: all noweb md_to_html

all: md_to_html
	@go build ${ARGS}

noweb: md_to_html
	@go build --tags noweb ${ARGS}

md_to_html:
	@python3 scripts/doc.py
