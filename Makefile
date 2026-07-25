
.PHONY: all noweb md_to_html test

all: md_to_html
	@go build ${ARGS}

test: all
	unshare -rn ./scripts/test.sh

noweb: md_to_html
	@go build --tags noweb ${ARGS}

md_to_html:
	@python3 scripts/doc.py
