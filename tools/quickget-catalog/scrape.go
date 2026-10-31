// Scraper handling: OSes whose get_ needs live page/JSON resolution.
// Implements the common patterns (json API, html links, yaml) natively
// — no bash delegation for these.
package main

import "errors"

// scrape resolves URL+hash for scrape-type OSes. Each case mirrors its
// get_<os> logic exactly.
func scrape(osName, release, edition, arch string) (string, string, error) {
	for _, resolve := range []func(string, string, string, string) (string, string, error){
		scrapeHTML, scrapeMirror, scrapeAPI0, scrapeUbuntuFamily, scrapeCloud,
	} {
		u, h, err := resolve(osName, release, edition, arch)
		if !errors.Is(err, errScrapeSkip) {
			return u, h, err
		}
	}
	return scrapeRescued(osName, release, edition, arch)
}
