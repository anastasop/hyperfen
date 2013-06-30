package hyperfen

import (
	"bytes"
	"crypto/sha1"
	"encoding/base64"
	"fmt"
	"html/template"
	"io"
	"io/ioutil"
	"net/http"
	"regexp"
	"strconv"
	"time"

	"appengine"
	"appengine/datastore"
	"appengine/mail"
	"appengine/urlfetch"

	"github.com/anastasop/gochess"
	"github.com/bmizerany/pat"
)

const (
	URL_REGEXP = "(https?|ftp)://[a-zA-Z0-9_@\\-]+([.:][a-zA-Z0-9_@-]+)*/?[a-zA-Z0-9_?,%#~&/\\-+=]+([:.][a-zA-Z0-9_?,%#~&/\\-+=]+)*"

	// salt for SHA1 hashes of PGN. It should be kept secret, but who cares for now
	salt = "I fight for the users"
)


var (
	templates *template.Template
	url_re *regexp.Regexp
)


func init() {
	url_re = regexp.MustCompile(URL_REGEXP)

	funcs := make(map[string]interface{})
	funcs["alreadyHTMLencoded"] = func(s string) template.HTML {
		return template.HTML(s)
	}
	templates = template.Must(template.New("templates").Funcs(funcs).ParseFiles("templates/hyperfen.tmpl"))

	mux := pat.New()
	mux.Get("/index", http.HandlerFunc(IndexHandler))
	mux.Post("/publish", http.HandlerFunc(PublishHandler))
	mux.Get("/publications/:key/:id", http.HandlerFunc(GameHandler))
	mux.Get("/publications/:key", http.HandlerFunc(PublicationHandler))
	mux.Get("/diagrams/:id", http.HandlerFunc(DiagramHandler))
	mux.Get("/", http.HandlerFunc(HomeHandler))
	http.Handle("/", mux)
}


func handleInternalError(c appengine.Context, w http.ResponseWriter, message string, err error) {
	c.Errorf("%s: %s", message, err)
	renderHtmlResponse(w, http.StatusInternalServerError, "error", ErrInternalError)
}


func renderHtmlResponse(w http.ResponseWriter, code int, template string, data interface{}) {
	w.Header().Set("Content-Type", "text/html")
	w.WriteHeader(code)
	templates.ExecuteTemplate(w, template, data)
}


func parsePGN(pgnReader io.Reader, limit int) ([]*ChessGame, error) {
	games := make([]*ChessGame, 0)
	parser := gochess.NewParser(pgnReader)
	var err error
	var pgngame *gochess.Game
	for {
		if pgngame, err = parser.NextGame(); err == nil && pgngame != nil  {
			if err = pgngame.ParseMovesText(); err == nil {
				g := new(ChessGame)
				g.Tags.Event = pgngame.Tags["Event"]
				g.Tags.Site = pgngame.Tags["Site"]
				g.Tags.Date = pgngame.Tags["Date"]
				g.Tags.Round, _ = strconv.Atoi(pgngame.Tags["Round"])
				g.Tags.White = pgngame.Tags["White"]
				g.Tags.Black = pgngame.Tags["Black"]
				g.Tags.Result = pgngame.Tags["Result"]
				g.Tags.ECO = pgngame.Tags["ECO"]
				g.Tags.Annotator = pgngame.Tags["Annotator"]
				g.Tags.WhiteElo, _ = strconv.Atoi(pgngame.Tags["WhiteElo"])
				g.Tags.BlackElo, _ = strconv.Atoi(pgngame.Tags["BlackElo"])
				g.PGN = []byte(pgngame.PGNText)
			
				board := gochess.NewBoard()
				for _, ply := range pgngame.Moves.Plies {
					if err = board.MakeMove(ply.SAN); err != nil {
						return nil, err
					}
					if ply.Comment != "" {
						var title, id string
						if san, whiteMove, moveNumber := board.LastMove(); whiteMove {
							title = fmt.Sprintf("%d. %s", moveNumber, san)
							id = fmt.Sprintf("%dw", moveNumber)
						} else {
							title = fmt.Sprintf("%d... %s", moveNumber, san)
							id = fmt.Sprintf("%db", moveNumber)
						}
						g.Cards = append(g.Cards, ChessCard{id, title, board.Fen(), []byte(htmlEncodedWithHrefsEnabled(ply.Comment))})
					}
				}
				g.Cards = append(g.Cards, ChessCard{"final", "Final Position", board.Fen(), []byte("")})
				games = append(games, g)
				if len(games) >= limit {
					return games, ErrGamesLimit
				}
			}
		}
		if err != nil || pgngame == nil {
			break
		}
	}
	return games, err
}


func PublishHandler(w http.ResponseWriter, r *http.Request) {
	c := appengine.NewContext(r)

	pgnTitle, pgnUrl, pgnDescription := r.FormValue("title"), r.FormValue("pgnurl"), r.FormValue("description")
	if pgnTitle == "" {
		renderHtmlResponse(w, http.StatusBadRequest, "error", ErrEmptyTitle)
		return
	} else if len(pgnTitle) > 80 {
		pgnTitle = pgnTitle[:80]
	}
	if pgnDescription == "" {
		renderHtmlResponse(w, http.StatusBadRequest, "error", ErrEmptyDescription)
		return
	} else if len(pgnTitle) > 1024 {
		pgnDescription = pgnDescription[:1024]
	}
	if  pgnUrl != "" && !url_re.MatchString(pgnUrl) {
		renderHtmlResponse(w, http.StatusBadRequest, "error", ErrInvalidPGNUrl)
		return
	}

	// first try to read the POSTed pgn file. The upload is done by appengine
	// so any errors are http 500 errors
	var pgnFileBytes []byte
	pgnFile, _, pgnFileError := r.FormFile("pgnfile")
	if pgnFileError == nil {
		defer pgnFile.Close()
		pgnFileBytes, pgnFileError = ioutil.ReadAll(pgnFile)
	}
	if pgnFileError != nil && pgnFileError != http.ErrMissingFile {
		handleInternalError(c, w, "ReadAll for POSTed pgn failed", pgnFileError)
		return
	}

	// you can't both upload a pgn and download another one from a url
	if pgnFile != nil && pgnUrl != "" {
		renderHtmlResponse(w, http.StatusBadRequest, "error", ErrMultiplePGNStream)
		return
	}

	// now try to download pgn from the url. In this case all errors
	// are http 503 bad gateway errors
	var pgnUrlBytes []byte
	var pgnUrlError error
	if pgnUrl != "" {
		var resp *http.Response
		resp, pgnUrlError = urlfetch.Client(c).Get(pgnUrl)
		if pgnUrlError == nil {
			defer resp.Body.Close()
			pgnUrlBytes, pgnUrlError = ioutil.ReadAll(resp.Body)
		}
	}
	if pgnUrlError != nil {
		renderHtmlResponse(w, http.StatusBadGateway, "error", ErrInternalError)
		return
	}

	var pgnBytes []byte
	if pgnFileBytes == nil && pgnUrlBytes == nil {
		renderHtmlResponse(w, http.StatusBadRequest, "error", ErrMissingPGNStream)
		return
	} else if pgnFileBytes != nil {
		pgnBytes = pgnFileBytes
		c.Infof("parsing uploaded png")
	} else {
		pgnBytes = pgnUrlBytes
		c.Infof("parsing downloaded png from '%s'", pgnUrl)
	}

	var redirectUrl string
	if games, err := parsePGN(bytes.NewBuffer(pgnBytes), kMaxParsedGames); err != nil {
		if err == ErrGamesLimit {
			renderHtmlResponse(w, http.StatusBadRequest, "error", ErrGamesLimit)
		} else {
			renderHtmlResponse(w, http.StatusInternalServerError, "error", ErrPGNParserFailure)
			msg := &mail.Message {
				Sender: "winwasher@gmail.com",
				Subject: "PGN parser failed",
				Body: fmt.Sprintf("Error: %s\nURL: %s\n", err, pgnUrl),
				Attachments: append([]mail.Attachment(nil), mail.Attachment{ Name: fmt.Sprintf("games-%d.pgn", time.Now().Unix()), Data: pgnBytes }),
			}
			if err := mail.SendToAdmins(c, msg); err != nil {
				c.Errorf("failed to send mail for pgn parser failure: %s", err)
			}
		}
		return
	} else if len(games) == 0 {
		c.Infof("WTF?? No games!!")
		renderHtmlResponse(w, http.StatusBadRequest, "error", ErrInvalidPGNData)
		return
	} else {
		if err := datastore.RunInTransaction(c, func(tc appengine.Context) error {
			publicationKey := datastore.NewKey(c, "Publication", base64encOfSHA1([]byte(pgnTitle), []byte(pgnDescription), pgnBytes), 0, nil)
			publication := Publication{
				publicationKey.StringID(),
				pgnTitle,
				[]byte(htmlEncodedWithHrefsEnabled(pgnDescription)),
				time.Now(),
				nil,
			}
			if _, err := datastore.Put(c, publicationKey, &publication); err != nil {
				return err
			}
			gameKeys := make([]*datastore.Key, len(games))
			for i := 0; i < len(gameKeys); i++ {
				gameKeys[i] = datastore.NewKey(c, "ChessGame", "", int64(i + 1), publicationKey)
				games[i].Key = int64(i + 1)
			}
			if _, err := datastore.PutMulti(c, gameKeys, games); err != nil {
				return err
			}
			if len(games) == 1 {
				redirectUrl = fmt.Sprintf("/publications/%s/%d", publicationKey.StringID(), gameKeys[0].IntID())
			} else {
				redirectUrl = fmt.Sprintf("/publications/%s", publicationKey.StringID())
			}
			return nil
		}, nil); err != nil {
			handleInternalError(c, w, "PublishHandler: datastore ops failed", err)
			return
		}
	}
	http.Redirect(w, r, redirectUrl, 303)
}


func HomeHandler(w http.ResponseWriter, r *http.Request) {
	renderHtmlResponse(w, http.StatusOK, "home", nil);
}


func GameHandler(w http.ResponseWriter, r *http.Request) {
	c := appengine.NewContext(r)
	key := r.URL.Query().Get(":key")
	id, _ := strconv.ParseInt(r.URL.Query().Get(":id"), 10, 64)
	view := r.URL.Query().Get("view")

	publicationKey := datastore.NewKey(c, "Publication", key, 0, nil)
	gameKey := datastore.NewKey(c, "ChessGame", "", id, publicationKey)
	var game ChessGame
	switch err := datastore.Get(c, gameKey, &game); true {
	case err == datastore.ErrNoSuchEntity:
		renderHtmlResponse(w, http.StatusBadRequest, "error", fmt.Errorf("There is no game with key %s/%d", key, id));
		return
	case err != nil:
		handleInternalError(c, w, "GameHandler: datastore ops failed", err)
		return
	}

	if view == "pgn" {
		w.Header().Set("Content-Type", "text/plain")
		w.Write(game.PGN)
		return
	}

	// lazy init opengraph data
	if game.OpenGraph.Title == "" {
		var lastAnnotatedCard ChessCard
		if len(game.Cards) > 1 {
			lastAnnotatedCard  = game.Cards[len(game.Cards) - 2]
		} else {
			lastAnnotatedCard  = game.Cards[len(game.Cards) - 1]
		}

		var diagramId int64
		diagram := &ChessDiagram {lastAnnotatedCard.Fen, fen2png(lastAnnotatedCard.Fen)}
		if key, err := datastore.Put(c, datastore.NewIncompleteKey(c, "ChessDiagram", nil), diagram); err != nil {
			handleInternalError(c, w, "GameHandler: datastore ops failed while writing opengraph data image", err)
			return
		} else {
			diagramId = key.IntID()
		}
		game.OpenGraph.Title = fmt.Sprintf("%s - %s %s %s %s", game.Tags.White, game.Tags.Black, game.Tags.Result, game.Tags.Event, game.Tags.Date)
		game.OpenGraph.Type = "article"
		game.OpenGraph.Image = fmt.Sprintf("http://hyper-fen.appspot.com/diagrams/%d", diagramId) 
		game.OpenGraph.ImageType = "image/png"
		game.OpenGraph.URL = fmt.Sprintf("http://hyper-fen.appspot.com/publications/%s/%d", key, id)
		if game.Tags.Annotator != "" {
			game.OpenGraph.Description = "Annotated by " + game.Tags.Annotator
		} else {
			game.OpenGraph.Description = "Not Annotated"
		}
		game.OpenGraph.Sitename = "HyperFen"
		game.OpenGraph.PublishedTime = time.Now() // this is wrong. to get the correct value read the publication
		game.OpenGraph.Author = game.Tags.Annotator
		if _, err := datastore.Put(c, gameKey, &game); err != nil {
			handleInternalError(c, w, "GameHandler: datastore ops failed while updating opengraph data", err)
			return
		}
	}
	renderHtmlResponse(w, http.StatusOK, "game", game);
}


func PublicationHandler(w http.ResponseWriter, r *http.Request) {
	c := appengine.NewContext(r)
	key := r.URL.Query().Get(":key")

	publicationKey := datastore.NewKey(c, "Publication", key, 0, nil)
	var publication Publication
	switch err := datastore.Get(c, publicationKey, &publication); true {
	case err == datastore.ErrNoSuchEntity:
		renderHtmlResponse(w, http.StatusBadRequest, "error", fmt.Errorf("There is no publication with key %q", key));
		return
	case err != nil:
		handleInternalError(c, w, "PublicationHandler: fetch Publication: datastore ops failed", err)
		return
	}

	q := datastore.NewQuery("ChessGame").Ancestor(publicationKey)
	games := make([]ChessGame, 0)
	if _, err := q.GetAll(c, &games); err != nil {
		handleInternalError(c, w, "PublicationHandler: fetch Publication.Games: datastore ops failed", err)
		return
	}

	publication.Games = games
	renderHtmlResponse(w, http.StatusOK, "publication", publication);
}


func DiagramHandler(w http.ResponseWriter, r *http.Request) {
	c := appengine.NewContext(r)
	id, _ := strconv.ParseInt(r.URL.Query().Get(":id"), 10, 64)

	diagramKey := datastore.NewKey(c, "ChessDiagram", "", id, nil)
	var diagram ChessDiagram
	switch err := datastore.Get(c, diagramKey, &diagram); true {
	case err == datastore.ErrNoSuchEntity:
		renderHtmlResponse(w, http.StatusBadRequest, "error", fmt.Errorf("There is no diagram with id %d", id));
		return
	case err != nil:
		handleInternalError(c, w, "PublicationHandler: fetch Diagram: datastore ops failed", err)
		return
	}

	w.Header().Set("Content-Type", "image/png")
	w.WriteHeader(http.StatusOK)
	w.Write(diagram.Pgnbytes)
}


func IndexHandler(w http.ResponseWriter, r *http.Request) {
	c := appengine.NewContext(r)

	q := datastore.NewQuery("Publication").Order("-Published")
	publications := make([]Publication, 0)
	if _, err := q.GetAll(c, &publications); err != nil {
		handleInternalError(c, w, "IndexHandler: fetch Publications: datastore ops failed", err)
		return
	}
	renderHtmlResponse(w, http.StatusOK, "index", publications);
}


func base64encOfSHA1(bodies ...[]byte) string {
	h := sha1.New()
	io.WriteString(h, salt)
	for _, body := range bodies {
		h.Write(body)
	}
	sum := h.Sum(nil)
	b := make([]byte, base64.URLEncoding.EncodedLen(len(sum)))
	base64.URLEncoding.Encode(b, sum)
	return string(b)
}


func htmlEncodedWithHrefsEnabled(s string) string {
	return url_re.ReplaceAllString(template.HTMLEscapeString(s), "<a href='$0' target='_blank'>$0</a>")
}
