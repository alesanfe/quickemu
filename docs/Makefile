include pandoc-man.mk

ifeq ($(PREFIX),)
    PREFIX := /usr/local
endif

datarootdir := $(PREFIX)/share
datadir := $(datarootdir)
mandir := $(datarootdir)/man
bindir :=  $(PREFIX)/bin

all: quickget.1 quickemu.1 quickemu_conf.5

clean:
	rm *.1 *.5

install_docs: all
	install -d $(DESTDIR)$(mandir)/man1
	install -d $(DESTDIR)$(mandir)/man5
	install -m 644 quickget.1 $(DESTDIR)$(mandir)/man1
	install -m 644 quickemu.1 $(DESTDIR)$(mandir)/man1
	install -m 644 quickemu_conf.5 $(DESTDIR)$(mandir)/man5
	rm -f $(DESTDIR)$(mandir)/man1/quickemu_conf.1

#   install -m 644 quickgui.1 $(DESTDIR)$(mandir)/man1

install_bins:
	install -d $(DESTDIR)$(bindir)
	install -m 755 ../bin/quickget $(DESTDIR)$(bindir)
	install -m 755 ../bin/quickemu $(DESTDIR)$(bindir)
	install -m 755 ../bin/quickreport $(DESTDIR)$(bindir)
	install -d $(DESTDIR)$(PREFIX)/libexec/quickemu
	install -m 755 ../libexec/quickemu/chunkcheck.py $(DESTDIR)$(PREFIX)/libexec/quickemu
	install -d $(DESTDIR)$(PREFIX)/lib
	cp -r ../lib/quickemu $(DESTDIR)$(PREFIX)/lib/
	install -d $(DESTDIR)$(datadir)/quickemu
	cp -r ../schemas ../catalog $(DESTDIR)$(datadir)/quickemu/

install: install_bins  install_docs

uninstall::
	rm -f $(DESTDIR)$(mandir)/man1/quickget.1
	rm -f $(DESTDIR)$(mandir)/man1/quickemu.1
	rm -f $(DESTDIR)$(mandir)/man5/quickemu_conf.5
	rm -f $(DESTDIR)$(bindir)/quickget
	rm -f $(DESTDIR)$(bindir)/quickemu
	rm -f $(DESTDIR)$(bindir)/quickreport
	rm -f $(DESTDIR)$(bindir)/macrecovery
	rm -f $(DESTDIR)$(PREFIX)/libexec/quickemu/chunkcheck.py
	rm -rf $(DESTDIR)$(PREFIX)/lib/quickemu
	rm -rf $(DESTDIR)$(datadir)/quickemu


.PHONY: all
