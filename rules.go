package main

import (
	"crypto/tls"
	"fmt"
	"github.com/PuerkitoBio/goquery"
	"github.com/chamilad/html-to-markdown"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var ruleOverrides = []md.Rule{
	// converter rule to convert github gists to markdown code blocks
	{
		//<figure name="3f51" id="3f51" class="graf graf--figure graf--iframe graf-after--p">
		// <script src="https://gist.github.com/chamilad/63cfa08c052e795c8e95bb7b43643f6a.js"></script>
		// </figure>
		Filter: []string{"script"},
		Replacement: func(content string, selec *goquery.Selection, options *md.Options) *string {
			codeContentType := ""
			codeContent := ""

			// check the src attribute
			src, exists := selec.Attr("src")
			if !exists {
				// if src cannot be found, nothing can be done
				printRedDot()
				return nil
			}

			// if src exists, check if it is a gist
			if !strings.HasPrefix(src, "https://gist.github") {
				return nil
			}

			// remove the js extension from the path
			// https://gist.github.com/chamilad/63cfa08c052e795c8e95bb7b43643f6a.js
			src = src[0 : len(src)-len(filepath.Ext(src))]
			//fmt.Printf("gist source path: %s", src)

			// get the content type from the html content
			skipTLS := strings.ToLower(os.Getenv("ALLOW_INSECURE")) == "true"
			tr := &http.Transport{
				TLSClientConfig: &tls.Config{InsecureSkipVerify: skipTLS},
			}

			client := &http.Client{Transport: tr}

			res, err := client.Get(src)
			if err != nil {
				printRedDot()
				return nil
			}

			htmlDoc, err := goquery.NewDocumentFromReader(res.Body)
			if err != nil {
				printRedDot()
				return nil
			}

			err = res.Body.Close()
			if err != nil {
				printRedDot()
				return nil
			}

			//<meta
			//class="js-ga-set"
			//name="dimension7"
			//content="xml"
			//>
			htmlDoc.Find("meta[name='dimension7']").Each(func(i int, selection *goquery.Selection) {
				ct, exists := selection.Attr("content")
				if !exists {
					printRedDot()
					return
				}

				// certain content types don't translate well to markdown
				if ct == "unknown" {
					codeContentType = ""
				} else if ct == "shell" {
					codeContentType = "bash"
				} else {
					codeContentType = ct
				}
			}) // content type reading done

			// get raw content
			rawsrc := fmt.Sprintf(
				"%s/raw",
				strings.Replace(src, "gist.github.com", "gist.githubusercontent.com", 1))

			rawres, err := client.Get(rawsrc)
			if err != nil {
				printRedDot()
				return nil
			}

			resBody, err := ioutil.ReadAll(rawres.Body)
			if err != nil {
				printRedDot()
				return nil
			}

			err = rawres.Body.Close()
			if err != nil {
				printRedDot()
				return nil
			}

			codeContent = string(resBody) // reading raw content done

			// if no raw content is read, return without rendering
			if len(codeContent) == 0 {
				return nil
			}

			// otherwise render a markdown code block with content type
			codeblock := fmt.Sprintf(
				"\n\n%s%s\n%s\n%s\n\n",
				options.Fence,
				codeContentType,
				codeContent,
				options.Fence)

			printDot()
			return md.String(codeblock)
		},

		AdvancedReplacement: nil,
	},

	// convert remaining br tags to new line chars
	{
		Filter: []string{"br"},
		Replacement: func(content string, selec *goquery.Selection, options *md.Options) *string {
			return md.String("\n")
		},
		AdvancedReplacement: nil,
	},

	// convert correctly any preformatted sections to unescaped multiline code blocks
	// this will also read any pre blocks found as consecutive siblings and collect them into one markdown code
	// block.
	{
		Filter: []string{"pre"},
		Replacement: func(content string, selec *goquery.Selection, options *md.Options) *string {
			// if this pre tag is already read, skip
			// this happens if pre blocks are found as siblings, the previous pre block processing would collect
			// all the next consecutive pre blocks into one code block and mark them as collected witb this
			// class.
			if selec.HasClass("m2h-collected") {
				return md.String("")
			}

			// read code for current pre block
			codeContent := ""
			readCodeContent(selec, &codeContent)

			// check if next element is a pre block, and read content if so
			nextSelec := selec.Next()
			for ; ; {
				if goquery.NodeName(nextSelec) != "pre" {
					break
				}

				// consecutive blocks usually mean an empty line in medium
				codeContent += "\n\n"

				// append content to single block
				readCodeContent(nextSelec, &codeContent)

				// mark pre block as collected
				nextSelec.AddClass("m2h-collected")

				// check next tag
				nextSelec = nextSelec.Next()
			}

			return md.String(fmt.Sprintf("\n\n%s\n%s\n%s\n\n", options.Fence, codeContent, options.Fence))
		},
		AdvancedReplacement: nil,
	},

	// convert slideshare links to proper iframe embeds
	{
		// <figure name="9af0" id="9af0" class="graf graf--figure graf--iframe graf-after--blockquote">
		// <iframe src="https://www.slideshare.net/slideshow/embed_code/key/8br68UFQtb7qpF" width="600" height="500"
		// frameborder="0" scrolling="no"></iframe>
		// </figure>
		Filter: []string{"iframe"},
		Replacement: func(content string, selec *goquery.Selection, options *md.Options) *string {
			src, exists := selec.Attr("src")
			if !exists || !strings.Contains(src, "slideshare.net") {
				return nil
			}

			return md.String(fmt.Sprintf(
				"<iframe src=\"%s\" width=\"595\" height=\"485\" frameborder=\"0\" marginwidth=\"0\" "+
					"marginheight=\"0\" scrolling=\"no\" style=\"border:1px solid #CCC; border-width:1px; "+
					"margin-bottom:5px; \" allowfullscreen> </iframe>\n",
				src))

		},
		AdvancedReplacement: nil,
	},

	// avoid escaping text unnecessarily, it's unlikely markdown directives will be in #text elements
	// in Medium posts
	{
		Filter: []string{"#text"},
		Replacement: func(content string, selec *goquery.Selection, options *md.Options) *string {
			text := selec.Text()
			if trimmed := strings.TrimSpace(text); trimmed == "" {
				return md.String("")
			}
			text = regexp.MustCompile(`\t+`).ReplaceAllString(text, " ")

			// replace multiple spaces by one space: dont accidentally make
			// normal text be indented and thus be a code block.
			text = regexp.MustCompile(`  +`).ReplaceAllString(text, " ")

			//text = escape.Markdown(text)
			return md.String(text)
		},
		AdvancedReplacement: nil,
	},

	// avoid `**text**` since it's not converted properly during md to html in hugo
	{
		Filter: []string{"strong", "b"},
		Replacement: func(content string, selec *goquery.Selection, options *md.Options) *string {
			if goquery.NodeName(selec.Parent()) != "code" {
				// eval other rules
				return nil
			}

			// if the parent is <code> do not put ** string
			return md.String(selec.Text())
		},
		AdvancedReplacement: nil,
	},

	// render figcaption correctly
	{
		Filter: []string{"img"},
		Replacement: func(content string, selec *goquery.Selection, options *md.Options) *string {
			// check if img and figcaption elements are present
			// figure
			//   > div.aspectRatioPlaceholder
			//     > img
			//   > figcaption

			// check if parent is a div.aspectRatioPlaceholder
			parent := selec.Parent()
			if goquery.NodeName(parent) != "div" {
				return nil
			}

			if !parent.HasClass("aspectRatioPlaceholder") {
				return nil
			}

			// check if parent has a sibling called figcaption
			parParent := parent.Parent()
			if goquery.NodeName(parParent) != "figure" {
				return nil
			}

			figcaption := parParent.Find("figcaption")
			if figcaption.Length() == 0 {
				return nil
			}

			imgSrc, _ := selec.Attr("src")
			fHtml := fmt.Sprintf(
				"<figure><img src=\"%s\"><figcaption>%s</figcaption></figure>\n",
				imgSrc,
				figcaption.Text())

			return md.String(fHtml)
		},
		AdvancedReplacement: nil,
	},

	// remove figcaption since this will be processed during the img tag processing
	{
		Filter: []string{"figcaption"},
		Replacement: func(content string, selec *goquery.Selection, options *md.Options) *string {
			return md.String("")
		},
		AdvancedReplacement: nil,
	},

	// handle tweets
	// have to use hugo shortcodes here since there is no easy way to generate a twitter embed code
	{
		Filter: []string{"blockquote"},
		Replacement: func(content string, selec *goquery.Selection, options *md.Options) *string {
			// check if a tweet
			if !selec.HasClass("twitter-tweet") {
				return nil
			}

			t, exists := selec.Find("a").Attr("href")
			if !exists {
				return nil
			}

			splits := strings.Split(t, "/")
			st := splits[len(splits)-1]
			return md.String(fmt.Sprintf("{{< tweet %s >}}", st))
		},
		AdvancedReplacement: nil,
	},

	// hrefed code
	{
		Filter: []string{"code"},
		Replacement: func(content string, selec *goquery.Selection, options *md.Options) *string {
			a := selec.Find("a")
			if a.Length() == 0 {
				return nil
			}

			linkT := a.Text()
			href := a.AttrOr("href", "")
			return md.String(fmt.Sprintf("[`%s`](%s)", linkT, href))
		},
		AdvancedReplacement: nil,
	},

	// empty hrefs
	{
		Filter: []string{"a"},
		Replacement: func(content string, selec *goquery.Selection, options *md.Options) *string {
			if strings.TrimSpace(selec.Text()) != "" {
				return nil
			}

			return md.String("")
		},
		AdvancedReplacement: nil,
	},
}

// readCodeContent reads the text of a given Selection, honouring the br tags found within the text. This is
// intended to be used to read content within pre blocks as the default rule in the converter library will
// ignore the br tags
// The read text will be appended to the string provided by the pointer
func readCodeContent(s *goquery.Selection, c *string) {
	s.Contents().Each(func(i int, selection *goquery.Selection) {
		name := goquery.NodeName(selection)

		if name == "br" {
			*c += "\n"
		} else {
			*c += selection.Text()
		}
	})
}
