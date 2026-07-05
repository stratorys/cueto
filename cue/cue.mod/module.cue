// CUE module root. Making cue/ a module turns it into an import root so policy
// packs (policy/*) and imported infra facts (infra) resolve, and so exports are
// portable. The schema/data package is imported as
// "github.com/stratorys/cue-diagram:diagram".
module: "github.com/stratorys/cue-diagram"
language: version: "v0.17.0"
