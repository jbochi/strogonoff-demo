package demo

import (
	"appengine"
	"appengine/datastore"
	"bytes"
	"crypto/sha1"
	"fmt"
	"http"
	"image"
	"image/jpeg"
	_ "image/png" // import so we can read PNG files.
	"io"
	"github.com/jbochi/strogonoff"
	"template"
	"os"
	"resize"
)

var (
	errorTemplate  = template.MustParseFile("templates/error.html", nil)
	viewTemplate   *template.Template // set up in init()
	uploadTemplate = template.MustParseFile("templates/upload.html", nil)
	
)

// Image is the type used to hold the image in the datastore.
type Image struct {
        Data []byte
}

func init() {
	http.HandleFunc("/", errorHandler(upload))	
	http.HandleFunc("/img", errorHandler(img))
	http.HandleFunc("/view", errorHandler(view))	
	viewTemplate = template.New(nil)
        viewTemplate.SetDelims("{{{", "}}}")
        if err := viewTemplate.ParseFile("templates/view.html"); err != nil {
                panic("can't parse view.html: " + err.String())
        }
}

// keyOf returns (part of) the SHA-1 hash of the data, as a hex string.
func keyOf(data []byte) string {
        sha := sha1.New()
        sha.Write(data)
        return fmt.Sprintf("%x", string(sha.Sum())[0:8])
}

func upload (w http.ResponseWriter, r *http.Request) {
        if r.Method != "POST" {
                // No upload; show the upload form.
                uploadTemplate.Execute(w, nil)
                return
        }

        f, _, err := r.FormFile("image")
        check(err)
        defer f.Close()

	msg  := r.Form["message"][0]

        // Grab the image data
        var buf bytes.Buffer
        io.Copy(&buf, f)
        i, _, err := image.Decode(&buf)
        check(err)

        // Resize if too large, for more efficient moustachioing.
        // We aim for less than 1200 pixels in any dimension; if the
        // picture is larger than that, we squeeze it down to 600.
        const max = 1200
        if b := i.Bounds(); b.Dx() > max || b.Dy() > max {
                // If it's gigantic, it's more efficient to downsample first
                // and then resize; resizing will smooth out the roughness.
                if b.Dx() > 2*max || b.Dy() > 2*max {
                        w, h := max, max
                        if b.Dx() > b.Dy() {
                                h = b.Dy() * h / b.Dx()
                        } else {
                                w = b.Dx() * w / b.Dy()
                        }
                        i = resize.Resample(i, i.Bounds(), w, h)
                        b = i.Bounds()
                }
                w, h := max/2, max/2
                if b.Dx() > b.Dy() {
                        h = b.Dy() * h / b.Dx()
                } else {
                        w = b.Dx() * w / b.Dy()
                }
                i = resize.Resize(i, i.Bounds(), w, h)
        }

        // Encode as a new JPEG image.
        buf.Reset()
        err = strogonoff.Encode(&buf, i, msg, nil)
        check(err)

        // Create an App Engine context for the client's request.
        c := appengine.NewContext(r)

        // Save the image under a unique key, a hash of the image.
        key := datastore.NewKey("Image", keyOf(buf.Bytes()), 0, nil)
        _, err = datastore.Put(c, key, &Image{buf.Bytes()})
        check(err)

        // Redirect to /view using the key.
        http.Redirect(w, r, "/view?id="+key.StringID(), http.StatusFound)
}

// view is the HTTP handler for viewing images (html page); it handles "/view".
func view(w http.ResponseWriter, r *http.Request) {
        viewTemplate.Execute(w, r.FormValue("id"))
}

// img is the HTTP handler for displaying images and painting moustaches;
// it handles "/img".
func img(w http.ResponseWriter, r *http.Request) {
        c := appengine.NewContext(r)
        key := datastore.NewKey("Image", r.FormValue("id"), 0, nil)
        im := new(Image)
        err := datastore.Get(c, key, im)
        check(err)

        m, _, err := image.Decode(bytes.NewBuffer(im.Data))
        check(err)

        w.Header().Set("Content-type", "image/jpeg")
        jpeg.Encode(w, m, nil)
}

// errorHandler wraps the argument handler with an error-catcher that
// returns a 500 HTTP error if the request fails (calls check with err non-nil).
func errorHandler(fn http.HandlerFunc) http.HandlerFunc {
        return func(w http.ResponseWriter, r *http.Request) {
                defer func() {
                        if err, ok := recover().(os.Error); ok {
                                w.WriteHeader(http.StatusInternalServerError)
                                errorTemplate.Execute(w, err)
                        }
                }()
                fn(w, r)
        }
}

// check aborts the current execution if err is non-nil.
func check(err os.Error) {
        if err != nil {
                panic(err)
        }
}
