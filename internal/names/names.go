// Package names generates random three-word names such as swift-dancing-meadow.
package names

import (
	"math/rand/v2"
	"strings"
)

var adjectives = strings.Fields(`
	amber bold brave bright brisk calm clever cosmic crisp curious
	daring eager early fancy fierce gentle golden grand happy hidden
	humble jolly keen kind lively lucky mellow merry mighty misty
	nimble noble patient plucky polished proud quick quiet rapid rustic
	serene shiny silent silver sleek smooth snappy solid steady sunny
	swift tidy tranquil vivid warm wild wise witty young zesty`)

var verbs = strings.Fields(`
	baking blooming bouncing building chasing climbing coasting dancing
	dashing diving drifting dreaming flying gliding glowing growing
	hiking hopping humming jumping laughing leaping marching painting
	racing roaming rolling rowing running sailing singing skating
	sliding soaring spinning sprinting surfing swaying swimming thinking
	turning wading walking wandering weaving whistling winding writing`)

var nouns = strings.Fields(`
	anchor aurora badger beacon breeze brook canyon cedar comet coral
	dune ember falcon fern fjord forest glacier grove harbor heron
	island lagoon lantern maple meadow mesa moon orchard otter pebble
	pine prairie quartz raven reef river robin sparrow spruce summit
	thunder tide timber valley willow`)

// Random returns a name like swift-dancing-meadow.
func Random() string {
	return pick(adjectives) + "-" + pick(verbs) + "-" + pick(nouns)
}

func pick(words []string) string {
	return words[rand.IntN(len(words))]
}
