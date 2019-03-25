// Copyright © 2019 NAME HERE <EMAIL ADDRESS>
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package cmd

import (
	"log"
	"net/http"
	"html/template"
	"regexp"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	restclient "k8s.io/client-go/rest"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	appsv1 "k8s.io/api/core/v1"

	_ "k8s.io/client-go/plugin/pkg/client/auth/oidc"
)

var bind string
var kubeconfig string

type Context struct {
	Namespaces map[string][]appsv1.Pod
}

type Page struct {
	Title string
	Body []byte
	Context []Context
}

func buildConfigFromFlags(masterUrl, kubeconfigPath string, context string) (*restclient.Config, error) {
		return clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
			&clientcmd.ClientConfigLoadingRules{ExplicitPath: kubeconfigPath},
				&clientcmd.ConfigOverrides{ CurrentContext: context, ClusterInfo: clientcmdapi.Cluster{Server: masterUrl} }).ClientConfig()
}

// serveCmd represents the serve command
var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "A brief description of your command",
	Long: `A longer description that spans multiple lines and likely contains examples
and usage of using your command. For example:

Cobra is a CLI library for Go that empowers applications.
This application is a tool to generate the needed files
to quickly create a Cobra application.`,
	Run: func(cmd *cobra.Command, args []string) {

		http.HandleFunc("/", makeHandler(indexHandler))

		log.Fatal(http.ListenAndServe(bind, nil))
	},
}

func indexHandler(w http.ResponseWriter, r *http.Request, title string) {
	// use the current context in kubeconfig
	rawConfig := clientcmd.GetConfigFromFileOrDie(kubeconfig)
	// contexts := reflect.ValueOf(rawConfig.Contexts).MapKeys()
	contexts := make([]string, 0, len(rawConfig.Contexts))
	for k := range rawConfig.Contexts {
		contexts = append(contexts, k)
	}
	log.Print(contexts)
	contextsMap := []Context{}

	for _, context := range contexts {
		config, err := buildConfigFromFlags("", kubeconfig, context)
		if err != nil {
			log.Print(err.Error())
		}
		// create the clientset
		clientset, err := kubernetes.NewForConfig(config)
		if err != nil {
			log.Print(err.Error())
		}

		namespaces, err := clientset.CoreV1().Namespaces().List(metav1.ListOptions{})
		if err != nil {
			log.Print(err.Error())
		}

		var namespaceKeys []string

		for _, keys := range namespaces.Items {
			namespaceKeys = append(namespaceKeys, keys.Name)
		}

		namespacesMap := make(map[string][]appsv1.Pod)

		for _, namespace := range namespaceKeys {
			pods, err := clientset.CoreV1().Pods(namespace).List(metav1.ListOptions{})
			if err != nil {
				log.Print(err.Error())
			} else {
				namespacesMap[namespace] = pods.Items
			}
		}

		contextsMap = append(contextsMap, Context{Namespaces: namespacesMap})
	}

	p := &Page{Title: "pods", Context: contextsMap}

	renderTemplate(w, "index", p)
}

var templates = template.Must(template.ParseFiles("web/templates/index.html"))

var validPath = regexp.MustCompile("(^/(edit|save|view)/([a-zA-Z0-9]+)$)|(^/$)")

func makeHandler(fn func(http.ResponseWriter, *http.Request, string)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		m := validPath.FindStringSubmatch(r.URL.Path)
		if m == nil {
			http.NotFound(w, r)
			return
		}
		fn(w, r, m[2])
	}
}

func loadPage(title string) (*Page, error) {
	body := []byte("nil")
	return &Page{Title: title, Body: body}, nil
}

func renderTemplate(w http.ResponseWriter, tmpl string, p *Page) {
	err := templates.ExecuteTemplate(w, tmpl+".html", p)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func homeDir() string {
	if h := os.Getenv("HOME"); h != "" {
		return h
	}
	return os.Getenv("USERPROFILE") // windows
}

func init() {
	rootCmd.AddCommand(serveCmd)

	// Here you will define your flags and configuration settings.

	// Cobra supports Persistent Flags which will work for this command
	// and all subcommands, e.g.:
	// serveCmd.PersistentFlags().String("foo", "", "A help for foo")

	// Cobra supports local flags which will only run when this command
	// is called directly, e.g.:
	serveCmd.Flags().StringVarP(&bind, "bind", "b", ":8080", "The address for which the http server will bind to")
	if home := homeDir(); home != "" {
		serveCmd.Flags().StringVarP(&kubeconfig, "kubeconfig", "k", filepath.Join(home, ".kube", "config"), "absolute path to the kubeconfig file")
	} else {
		serveCmd.Flags().StringVarP(&kubeconfig, "kubeconfig", "k", "", "absolute path to the kubeconfig file")
	}
}

