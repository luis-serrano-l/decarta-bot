package bot

import (
	"bytes"
	"encoding/base64"
	"flag"
	"fmt"
	"html"
	"os"
	"strings"
	"testing"
)

// The README shows docs/demo.svg, an animated replay of a conversation
// with the bot. TestDemoReplay generates it from what the handler really
// sends, so the picture cannot drift from the bot: when a reply changes,
// the test fails until the file is regenerated with
//
//	go test ./internal/bot -run TestDemoReplay -update
var updateDemo = flag.Bool("update", false, "rewrite docs/demo.svg from the bot's current replies")

const (
	demoFile = "../../docs/demo.svg"
	// demoSite is the real site, where the menu of the replay is published
	// by hand, so the link and the QR code in the picture work.
	demoSite = "https://luis-serrano-l.github.io/decarta-site"

	demoMenu = `CATEGORY: Antipasti
- ITEM: Bruschetta
  PRICE: 6.50
  DESCRIPTION: Tomato, basil, olive oil
- ITEM: Burrata
  PRICE: 9.00

CATEGORY: Pasta
- ITEM: Carbonara
  PRICE: 12.50
- ITEM: Cacio e pepe
  PRICE: 11.00

CATEGORY: Dolci
- ITEM: Tiramisù
  PRICE: 6.00`
)

// demoBubble is one message as drawn in a frame.
type demoBubble struct {
	fromUser bool
	text     string
	rows     [][]Button
	photo    []byte
}

// demoFrame is the chat at one moment, shown for a number of seconds.
type demoFrame struct {
	bubbles []demoBubble
	pressed string // label of the button being tapped, if any
	seconds float64
}

// demoChat replays a conversation and records a frame after each step.
type demoChat struct {
	f      *fixture
	order  []demoBubble // user messages, and bot messages by id only
	botIDs []int        // parallel to order; 0 for a user message
	seen   int
	frames []demoFrame
}

// sync appends the bot messages sent since the last call.
func (c *demoChat) sync() {
	c.f.tg.mu.Lock()
	defer c.f.tg.mu.Unlock()
	msgs := c.f.tg.msgs[allowedID]
	for _, m := range msgs[c.seen:] {
		c.order = append(c.order, demoBubble{})
		c.botIDs = append(c.botIDs, m.id)
	}
	c.seen = len(msgs)
}

// frame records the chat as it is now. Bot messages are read again each
// time, because the handler edits them in place.
func (c *demoChat) frame(seconds float64, pressed string) {
	c.f.tg.mu.Lock()
	defer c.f.tg.mu.Unlock()
	byID := map[int]*sentMessage{}
	for _, m := range c.f.tg.msgs[allowedID] {
		byID[m.id] = m
	}
	fr := demoFrame{pressed: pressed, seconds: seconds}
	for i, b := range c.order {
		if m := byID[c.botIDs[i]]; m != nil {
			b = demoBubble{text: m.text, rows: m.rows}
			if m.photo {
				b.photo = m.file
			}
		}
		fr.bubbles = append(fr.bubbles, b)
	}
	c.frames = append(c.frames, fr)
}

// say sends a message from the user: one frame with the message alone,
// then one with the bot's answer.
func (c *demoChat) say(text string, readSeconds, answerSeconds float64) {
	c.order = append(c.order, demoBubble{fromUser: true, text: text})
	c.botIDs = append(c.botIDs, 0)
	c.frame(readSeconds, "")
	c.f.send(allowedID, text)
	c.sync()
	c.frame(answerSeconds, "")
}

// tap presses a button: one frame with the button held, then the result.
func (c *demoChat) tap(label string, answerSeconds float64) {
	c.frame(0.7, label)
	c.f.press(allowedID, label)
	c.sync()
	c.frame(answerSeconds, "")
}

func TestDemoReplay(t *testing.T) {
	f := newFixture(t)
	f.h.opts.SiteBaseURL = demoSite
	c := &demoChat{f: f}

	c.frame(0.8, "")
	c.say("/start", 0.9, 2.4)
	c.say(demoMenu, 1.5, 3)
	c.tap("Publish", 3)
	c.tap("Get QR code", 5)

	last := c.frames[len(c.frames)-1].bubbles
	if qr := last[len(last)-1]; len(qr.photo) == 0 {
		t.Fatalf("the replay does not end with the QR code: %q", qr.text)
	}

	got := renderDemo(c.frames)
	if *updateDemo {
		if err := os.WriteFile(demoFile, got, 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(demoFile)
	if err != nil {
		t.Fatalf("%v\ngenerate it with: go test ./internal/bot -run TestDemoReplay -update", err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("%s no longer matches what the bot sends; regenerate it with:\n"+
			"go test ./internal/bot -run TestDemoReplay -update", demoFile)
	}
}

const (
	demoWidth  = 400
	demoHeight = 760
	demoHeader = 60
)

// demoStyle uses the colours of the site. Frames are stacked, each on an
// opaque background, and appear in turn; without animation support (or
// with reduced motion) all are visible, so the last one, the whole
// conversation, is what shows.
const demoStyle = `
.f{animation-duration:%.1fs;animation-iteration-count:infinite;animation-timing-function:step-end}
@media (prefers-reduced-motion:reduce){.f{animation:none}}
.chat{box-sizing:border-box;height:100%%;display:flex;flex-direction:column;justify-content:flex-end;gap:8px;
padding:12px;overflow:hidden;font:14px/1.38 system-ui,-apple-system,"Segoe UI",Roboto,sans-serif;color:#2a211b}
.m{box-sizing:border-box;max-width:86%%;padding:8px 12px;border-radius:16px;white-space:pre-wrap;overflow-wrap:anywhere;flex:none}
.u{align-self:flex-end;background:#b23a0a;color:#fff;border-bottom-right-radius:4px}
.b{align-self:flex-start;background:#fff;border:1px solid #eeddd0;border-bottom-left-radius:4px}
.b img{display:block;width:190px;height:190px;margin:0 0 8px;border-radius:8px}
.r{display:flex;gap:6px;margin-top:8px}
.k{flex:1;padding:7px 4px;border-radius:10px;background:#fde5d4;color:#b23a0a;font-weight:600;text-align:center;white-space:nowrap}
.k.p{background:#b23a0a;color:#fff}
.h{font:600 17px system-ui,-apple-system,"Segoe UI",Roboto,sans-serif;fill:#2a211b}
.s{font:13px system-ui,-apple-system,"Segoe UI",Roboto,sans-serif;fill:#6b5d52}
`

// renderDemo draws the frames as one animated SVG. It needs no script and
// no external file, which is all an image in a README is allowed.
func renderDemo(frames []demoFrame) []byte {
	var total float64
	for _, fr := range frames {
		total += fr.seconds
	}

	var b bytes.Buffer
	fmt.Fprintf(&b, `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 %d %d" width="%d" height="%d" role="img" aria-labelledby="t">`+"\n",
		demoWidth, demoHeight, demoWidth, demoHeight)
	b.WriteString(`<title id="t">Replay of a chat with decarta-bot: /start, a menu sent as text, the preview, Publish, and the QR code of the published page</title>` + "\n")
	b.WriteString("<style>")
	fmt.Fprintf(&b, demoStyle, total)
	var start float64
	for i, fr := range frames {
		if i > 0 {
			fmt.Fprintf(&b, "@keyframes f%d{0%%{opacity:0}%.2f%%{opacity:1}100%%{opacity:1}}\n", i, 100*start/total)
		}
		start += fr.seconds
	}
	b.WriteString("</style>\n")
	fmt.Fprintf(&b, `<clipPath id="c"><rect width="%d" height="%d" rx="28"/></clipPath>`+"\n", demoWidth, demoHeight)
	b.WriteString(`<g clip-path="url(#c)">` + "\n")

	for i, fr := range frames {
		if i == 0 {
			b.WriteString(`<g>`)
		} else {
			fmt.Fprintf(&b, `<g class="f" style="animation-name:f%d">`, i)
		}
		fmt.Fprintf(&b, `<rect width="%d" height="%d" fill="#fffaf4"/>`, demoWidth, demoHeight)
		fmt.Fprintf(&b, `<foreignObject x="0" y="%d" width="%d" height="%d"><div xmlns="http://www.w3.org/1999/xhtml" class="chat">`,
			demoHeader, demoWidth, demoHeight-demoHeader)
		for _, m := range fr.bubbles {
			class := "m b"
			if m.fromUser {
				class = "m u"
			}
			fmt.Fprintf(&b, `<div class="%s">`, class)
			if len(m.photo) > 0 {
				fmt.Fprintf(&b, `<img alt="QR code" src="data:image/png;base64,%s"/>`, base64.StdEncoding.EncodeToString(m.photo))
			}
			b.WriteString(html.EscapeString(m.text))
			for _, row := range m.rows {
				b.WriteString(`<div class="r">`)
				for _, btn := range row {
					class := "k"
					if btn.Text == fr.pressed {
						class = "k p"
					}
					fmt.Fprintf(&b, `<div class="%s">%s</div>`, class, html.EscapeString(btn.Text))
				}
				b.WriteString(`</div>`)
			}
			b.WriteString(`</div>`)
		}
		b.WriteString("</div></foreignObject></g>\n")
	}

	// The header is drawn once, above every frame.
	fmt.Fprintf(&b, `<rect width="%d" height="%d" fill="#fde5d4"/>`, demoWidth, demoHeader)
	b.WriteString(`<circle cx="36" cy="30" r="18" fill="#b23a0a"/>`)
	b.WriteString(`<text x="36" y="36" text-anchor="middle" style="font:700 17px system-ui,sans-serif;fill:#fff">d</text>`)
	b.WriteString(`<text class="h" x="66" y="27">decarta-bot</text><text class="s" x="66" y="46">bot</text>` + "\n")
	fmt.Fprintf(&b, `</g><rect x="0.5" y="0.5" width="%d" height="%d" rx="28" fill="none" stroke="#b79f8c"/>`+"\n", demoWidth-1, demoHeight-1)
	b.WriteString("</svg>\n")
	return []byte(strings.ReplaceAll(b.String(), "\r", ""))
}
