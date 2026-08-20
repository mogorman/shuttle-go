UUID := mouse-position@shuttle-go.dev
VERSION := $(shell jq -r '.version' metadata.json)
TARBALL := $(UUID)-v$(VERSION).tar.gz

.PHONY: all clean install

all: $(TARBALL)

$(TARBALL): extension.js metadata.json
	tar -czf $@ extension.js metadata.json

# Install into the current user's GNOME extensions directory and enable it.
install: $(TARBALL)
	gnome-extensions install --enable $(TARBALL)

clean:
	rm -f $(TARBALL)
