// Command drive-files is an example Viper plugin: an app route that lists
// the signed-in user's Google Drive files, using the Drive account the user
// connected in the app (viper.connections.start).
package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/Polyphron-AI/viper-plugin/kit"
	pb "github.com/Polyphron-AI/viper-plugin/proto"
)

// driveScope is the one scope the manifest's connection names.
const driveScope = "https://www.googleapis.com/auth/drive.readonly"

// settings is what a project sets for the plugin.
type settings struct {
	// APIBase is Google's API address; a test points it at a fake.
	APIBase string `json:"apiBase"`
}

// web calls Google with a timeout, so a slow Google can't hold a call.
var web = &http.Client{Timeout: 15 * time.Second}

// main serves the plugin.
func main() {
	err := kit.Serve(&kit.Plugin{
		Description: &pb.Description{
			Id:      "drivefiles",
			Version: "1.0.0",
			Title:   "Drive files",
			Routes:  []*pb.Route{{Method: "GET", Path: "files", Summary: "Lists the caller's newest Drive files."}},
			Settings: []*pb.Field{{
				Key:         "apiBase",
				Label:       "Google API address",
				Kind:        "text",
				Description: "Leave empty for Google.",
			}},
		},
		Routes: map[string]kit.RouteFunc{"GET files": files},
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// files answers with the caller's 20 newest Drive files, or says what to
// do when they aren't signed in or haven't connected Google.
func files(c *kit.Call, r *pb.RouteRequest) (*pb.RouteReply, error) {
	user, ok := strings.CutPrefix(
		c.Caller,
		"user/",
	)
	if !ok {
		return kit.JSON(401, map[string]string{"message": "Sign in first."})
	}

	token, _, err := c.AccessToken(
		user,
		"google",
		driveScope,
	)

	switch status.Code(err) {
	case codes.OK:
	case codes.NotFound, codes.FailedPrecondition:
		return kit.JSON(409, map[string]any{"message": "Connect Google Drive first.", "connect": "google"})
	default:
		return nil, err
	}

	var s settings

	if err := c.Settings(&s); err != nil {
		return nil, err
	}

	if s.APIBase == "" {
		s.APIBase = "https://www.googleapis.com"
	}

	list, err := driveFiles(
		s.APIBase,
		token,
	)
	if err != nil {
		return nil, err
	}

	return kit.JSON(200, map[string]any{"files": list})
}

// file is one Drive file, as the route answers it.
type file struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	MimeType string `json:"mimeType"`
}

// driveFiles asks Drive for the newest 20 files the token can see.
func driveFiles(base, token string) ([]file, error) {
	q := url.Values{
		"pageSize": {"20"},
		"orderBy":  {"modifiedTime desc"},
		"fields":   {"files(id,name,mimeType)"},
	}
	req, err := http.NewRequest(
		"GET",
		strings.TrimRight(base, "/")+"/drive/v3/files?"+q.Encode(),
		nil,
	)
	if err != nil {
		return nil, err
	}

	req.Header.Set(
		"Authorization",
		"Bearer "+token,
	)

	res, err := web.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	if res.StatusCode != 200 {
		return nil, fmt.Errorf("drive answered %s", res.Status)
	}

	var out struct{ Files []file }
	err = json.NewDecoder(res.Body).Decode(&out)

	return out.Files, err
}
