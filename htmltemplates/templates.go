// Package htmltemplates a plugin to provide html body.
package htmltemplates

import (
	"bytes"
	"html/template"
)

type statusMap struct {
	Status  int16
	Message string
}

// GetErrorBody build error response HTML body.
func GetErrorBody(status int16) ([]byte, error) {
	message := getStatusMessage(status)

	params := statusMap{
		Status:  status,
		Message: message,
	}

	temp, err := template.New("error body").Parse(templateString)
	if err != nil {
		return nil, err
	}

	var buffer bytes.Buffer

	err = temp.Execute(&buffer, params)
	if err != nil {
		return nil, err
	}

	return buffer.Bytes(), nil
}

const templateString = `
<html lang="en">

  <head>
    <meta charset="utf-8">
    <meta name="viewport"
      content="width=device-width, initial-scale=1">
    <meta name="robots"
      content="noindex, nofollow">
    <title>{{ .Message }}</title>
    <style>
      html,
      body {
        background-color: #222526;
        color: #fff;
        font-family: 'Nunito', sans-serif;
        font-weight: 100;
        height: 100vh;
        margin: 0;
        font-size: 0
      }

      .full-height {
        height: 100vh
      }

      .flex-center {
        align-items: center;
        display: flex;
        justify-content: center
      }

      .position-ref {
        position: relative
      }

      .code {
        border-right: 2px solid;
        font-size: 26px;
        padding: 0 10px 0 15px;
        text-align: center
      }

      .message {
        font-size: 18px;
        text-align: center;
        padding: 10px
      }
    </style>
  </head>

  <body>
    <div class="flex-center position-ref full-height">
      <div>
        <div class="flex-center">
          <div class="code">
            {{ .Status }}
          </div>
          <div class="message" data-l10n="">
            {{ .Message }}
          </div>
        </div>
      </div>
    </div>
    <script>
      if (navigator.language.substring(0, 2).toLowerCase() !== 'en') {
        ((s, p) => { // localize the page (details here - https://github.com/tarampampam/error-pages/tree/master/l10n)
          s.src = 'https://cdn.jsdelivr.net/gh/tarampampam/error-pages@2/l10n/l10n.min.js'; // '../l10n/l10n.js';
          s.async = s.defer = true;
          s.addEventListener('load', () => p.removeChild(s));
          p.appendChild(s);
        })(document.createElement('script'), document.body);
      }
    </script>
  </body>

</html>
`
