package main

import (
	"crypto/tls"
	"errors"
	"fmt"
	"github.com/PuerkitoBio/goquery"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

type Post struct {
	DOM                   *goquery.Document
	Title, Author, Body   string
	Date, Lastmod         string
	Subtitle, Description string
	Canonical, FullURL    string
	FeaturedImage         string
	Images                []*Image
	Tags                  []string
	Draft                 bool
	MdFilename            string
	HTMLFileName          string
}

// PruneMediumSpecifics removes unwanted elements in the HTML document
func (p *Post) PruneMediumSpecifics() {
	//fix the big previews boxes for URLS, we don't need the description and other stuff
	//html2md cannot handle them (is a div of <a> that contains <strong><br><em>...)
	p.DOM.Find(".graf .markup--mixtapeEmbed-anchor").Each(func(i int, link *goquery.Selection) {
		title := link.Find("strong")
		if title == nil {
			return
		}

		//remove the <strong> <br> and <em>
		link.Empty()

		link.SetText(strings.TrimSpace(title.Text()))
	})

	//remove the empty URLs (which is the thumbnail on medium)
	p.DOM.Find(".graf a.mixtapeImage").Each(func(i int, selection *goquery.Selection) {
		selection.Remove()
	})

	//remove the TITLE, it is already as metadata in the markdown
	p.DOM.Find("h3.graf--title").Each(func(i int, selection *goquery.Selection) {
		selection.Remove()
	})

	p.DOM.Find("h1").Each(func(i int, selection *goquery.Selection) {
		selection.Remove()
	})
}

// NewImage creates an Image struct based on the given DOM element
func (p *Post) NewImage(dom *goquery.Selection, i int) (*Image, error) {
	imgSrc, exists := dom.Attr("src")
	if !exists {
		return nil, errors.New("invalid img def, no src found")
	}

	ext := filepath.Ext(imgSrc)
	if len(ext) < 2 {
		ext = ".jpg"
	}

	fileNamePrefix, err := p.GetFileNamePrefix()
	if err != nil {
		return nil, err
	}

	imgFilename := fmt.Sprintf("%s_%d%s", fileNamePrefix, i, ext)
	img := &Image{
		MediumURL: imgSrc,
		FileName:  imgFilename,
	}

	// all successful, attach a reference
	p.Images = append(p.Images, img)
	return img, nil
}

// GetFileNamePrefix returns just the markdown filename without the extension
// of a given Post. This can be used for naming related artifacts.
func (p *Post) GetFileNamePrefix() (string, error) {
	if len(strings.TrimSpace(p.MdFilename)) == 0 {
		return "", fmt.Errorf("empty filename")
	}

	if filepath.Ext(p.MdFilename) != MarkdownFileExtension {
		return "", fmt.Errorf("invalid filename set: %s", p.MdFilename)
	}

	return strings.TrimSuffix(p.MdFilename, MarkdownFileExtension), nil
}

// SetCanonicalName finds the url of the given Post and extracts the medium url
// to be added as an Alias in Hugo FrontMatter
func (p *Post) SetCanonicalName() {
	canonical := p.DOM.Find(".p-canonical")
	if canonical != nil {
		var exists bool
		//https://coder.today/a-b-tests-developers-manual-f57f5c1a492
		p.FullURL, exists = canonical.Attr("href")
		if exists && len(p.FullURL) > 0 {
			pieces := strings.Split(p.FullURL, "/")
			if len(pieces) > 2 {
				//a-b-tests-developers-manual-f57f5c1a492
				p.Canonical = pieces[len(pieces)-1] //we only need the last part
			}
		}
	}
}

// FixSelfLinks searches for links in the page that refers to other posts by
// the same author (provided username) and changes them to point to local
// relative URLs so that they will point to the Hugo hosted pages
func (p *Post) FixSelfLinks(username string) error {
	username = strings.TrimSpace(username)
	if len(username) == 0 {
		return fmt.Errorf("invalid username")
	}

	mediumBaseUrl := fmt.Sprintf("%s/@%s", "https://medium.com", username)

	anchors := p.DOM.Find(".markup--anchor")
	if anchors.Length() != 0 {
		anchors.Each(func(i int, aDomElement *goquery.Selection) {
			original, has := aDomElement.Attr("href")
			if !has {
				return
			}

			if strings.Contains(original, mediumBaseUrl) {
				replaced := strings.TrimPrefix(original, mediumBaseUrl)
				aDomElement.SetAttr("href", replaced)
				aDomElement.SetAttr("data-href", replaced)
			}
		})
	}

	return nil
}

// PopulateTags will collect the medium tags of the post by downloading the
// page and reading the tag values directly. This can only be done to published
// posts so drafts will be ignored.
func (p *Post) PopulateTags() error {
	if p.Draft {
		return nil
	}

	skipTLS := strings.ToLower(os.Getenv("ALLOW_INSECURE")) == "true"
	tr := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: skipTLS},
	}

	client := &http.Client{Transport: tr}

	res, err := client.Get(p.FullURL)
	if err != nil {
		return err
	}

	doc, err := goquery.NewDocumentFromReader(res.Body)
	if err != nil {
		return err
	}

	err = res.Body.Close()
	if err != nil {
		return err
	}

	doc.Find("ul>li>a[href^='/tag']").Each(func(i int, selection *goquery.Selection) {
		p.Tags = append(p.Tags, selection.Text())
	})

	return nil
}
