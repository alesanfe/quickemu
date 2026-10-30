// scrape_new.go — orchestrator for the second catalog-expansion
// batch. Each mechanism group lives in its own file; sub-resolvers
// return errScrapeSkip for families they do not handle.
package main

import (
	"errors"
	"fmt"
)

// scrapeRescued handles the second expansion batch; called from scrape's
// default branch.
func scrapeRescued(osName, release, edition, arch string) (string, string, error) {
	for _, resolve := range []func(string, string, string, string) (string, string, error){
		scrapeGH, scrapeSF, scrapeIndex, scrapeAPI, scrapeUbuntu,
	} {
		u, h, err := resolve(osName, release, edition, arch)
		if !errors.Is(err, errScrapeSkip) {
			return u, h, err
		}
	}
	return "", "", fmt.Errorf("scrape not implemented for %s", osName)
}
