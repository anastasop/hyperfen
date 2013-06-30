package hyperfen

import (
	"errors"
	"time"
)

const (
	kMaxParsedGames = 50
)

var (
	ErrEmptyTitle = errors.New("title is empty")
	ErrEmptyDescription = errors.New("description is empty")
	ErrMissingPGNStream = errors.New("you must upload a PGN file or specify a url to download a PGN file")
	ErrMultiplePGNStream= errors.New("you must upload a PGN file or specify a url to download a PGN file, not both")
	ErrInvalidPGNUrl = errors.New("the PGN url is invalid")
	ErrInvalidPGNData = errors.New("not valid PGN")
	ErrInternalError = errors.New("Server error. Please try again later")
	ErrGamesLimit = errors.New("the PGN must contain at most 50 games")
	ErrPGNParserFailure = errors.New("Sorry, but we cannot parse the provided PGN")
)

type ChessTags struct {
	Event string
	Site string
	Date string
	Round int
	White string
	Black string
	Result string
	ECO string
	Annotator string
	WhiteElo int
	BlackElo int
}

type ChessCard struct {
	ID string
	Title     string
	Fen     string
	Description []byte
}

type ChessGame struct {
	Key int64
	Tags ChessTags
	Cards []ChessCard
	PGN []byte

	OpenGraph OpenGraphData
}

type ChessDiagram struct {
	Fen string
	Pgnbytes []byte
}

type OpenGraphData struct {
	Title string
	Type string
	Image string
	ImageType string
	URL string
	Description string
	Sitename string

	PublishedTime time.Time
	Author string
}

type Publication struct {
	Key string
	Title string
	Description []byte
	Published time.Time
	Games []ChessGame `datastore:"-"`
}
