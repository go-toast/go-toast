package toast

import (
	"bytes"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"text/template"

	"github.com/nu7hatch/gouuid"
	"golang.org/x/sys/windows/registry"
)

var toastTemplate *template.Template

func init() {
	toastTemplate = template.New("toast")
	toastTemplate.Parse(`
[Windows.UI.Notifications.ToastNotificationManager, Windows.UI.Notifications, ContentType = WindowsRuntime] | Out-Null
[Windows.UI.Notifications.ToastNotification, Windows.UI.Notifications, ContentType = WindowsRuntime] | Out-Null
[Windows.Data.Xml.Dom.XmlDocument, Windows.Data.Xml.Dom.XmlDocument, ContentType = WindowsRuntime] | Out-Null

$APP_ID = '{{if .AppID}}{{.AppID}}{{else}}io.github.go-toast.toast{{end}}'

$template = @"
<toast>
    <visual>
        <binding template="ToastGeneric">
            {{if .Icon}}
            <image placement="appLogoOverride" src="{{.Icon}}" />
            {{end}}
            {{if .Title}}
            <text>{{.Title}}</text>
            {{end}}
            {{if .Message}}
            <text>{{.Message}}</text>
            {{end}}
        </binding>
    </visual>
    {{if .Actions}}
    <actions>
        {{range .Actions}}
        <action activationType="{{.Type}}" content="{{.Label}}" arguments="{{.Arguments}}" />
        {{end}}
    </actions>
    {{end}}
</toast>
"@

$xml = New-Object Windows.Data.Xml.Dom.XmlDocument
$xml.LoadXml($template)
$toast = New-Object Windows.UI.Notifications.ToastNotification $xml
[Windows.UI.Notifications.ToastNotificationManager]::CreateToastNotifier($APP_ID).Show($toast)
    `)
}

type Notification struct {
	// The name of your app. This value shows up in Windows 10's Action Centre, so make it
	// something readable for your users. It can contain spaces, however special characters
	// (eg. é) are not supported.
	AppID   string

	// The main title/heading for the toast notification.
	Title   string

	// The single/multi line message to display for the toast notification.
	Message string

	// An optional path to an image on the OS to display to the left of the title & message.
	Icon    string

	// Optional action buttons to display below the notification title & message.
	Actions []Action

	// Should the notification persist in Action Center?
	Persist bool
}

// Defines an actionable button.
// See https://msdn.microsoft.com/en-us/windows/uwp/controls-and-patterns/tiles-and-notifications-adaptive-interactive-toasts for more info.
//
// Only protocol type action buttons are actually useful, as there's no way of receiving feedback from the
// user's choice. Examples of protocol type action buttons include: "bingmaps:?q=sushi" to open up Windows 10's
// maps app with a pre-populated search field set to "sushi".
//
//     toast.Action{"protocol", "Open Maps", "bingmaps:?q=sushi"}
type Action struct {
	Type      string
	Label     string
	Arguments string
}

func (n *Notification) buildXML() (string, error) {
	var out bytes.Buffer
	err := toastTemplate.Execute(&out, n)
	if err != nil {
		return "", err
	}
	return out.String(), nil
}

// Builds the Windows PowerShell script & invokes it, causing the toast to display.
//
// Note: Running the PowerShell script is by far the slowest process here, and can take a few
// seconds in some cases.
//
//     notification := toast.Notification{
//         AppID: "Example App",
//         Title: "My notification",
//         Message: "Some message about how important something is...",
//         Icon: "go.png",
//         Actions: []toast.Action{
//             {"protocol", "I'm a button", ""},
//             {"protocol", "Me too!", ""},
//         },
//     }
//     err := notification.Push()
//     if err != nil {
//         log.Fatalln(err)
//     }
func (n *Notification) Push() error {
	if n.Persist {
		// Create a key for the AppID's persistence setting.
		k, _, _ := registry.CreateKey(registry.CURRENT_USER, `SOFTWARE\Microsoft\Windows\CurrentVersion\Notifications\Settings\`+n.AppID, registry.ALL_ACCESS)
		// Set the correct registry value.
		k.SetDWordValue("ShowInActionCenter", uint32(1))
		k.Close()
	} else {
		registry.DeleteKey(registry.CURRENT_USER, `SOFTWARE\Microsoft\Windows\CurrentVersion\Notifications\Settings\`+n.AppID)
	}

	xml, _ := n.buildXML()
	return invokeTemporaryScript(xml)
}

func invokeTemporaryScript(content string) error {
	id, _ := uuid.NewV4()
	file := filepath.Join(os.TempDir(), id.String()+".ps1")
	defer os.Remove(file)
	err := ioutil.WriteFile(file, []byte(content), 0600)
	if err != nil {
		return err
	}
	if err = exec.Command("PowerShell", "-ExecutionPolicy", "Bypass", "-File", file).Run(); err != nil {
		return err
	}
	return nil
}
