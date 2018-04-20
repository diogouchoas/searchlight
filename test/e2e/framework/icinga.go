package framework

import (
	"fmt"
	"time"

	"github.com/appscode/searchlight/pkg/icinga"
	. "github.com/onsi/gomega"
	"github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func (f *Framework) EventuallyIcingaAPI() GomegaAsyncAssertion {
	return Eventually(
		func() error {
			if f.icingaClient.Check().Get(nil).Do().Status == 200 {
				PrintSeparately("Connected to icinga api")
				return nil
			}
			fmt.Println(string(f.icingaClient.Check().Get(nil).Do().ResponseBody))
			fmt.Println("Waiting for icinga to start")
			return errors.New("icigna is not ready")
		},
		time.Minute*10,
		time.Second*10,
	)
}

func (f *Framework) GetIcingaApiAuth(meta metav1.ObjectMeta) (*icinga.Config, error) {
	secret, err := f.kubeClient.CoreV1().Secrets(meta.Namespace).Get(meta.Name, metav1.GetOptions{})
	if err != nil {
		return nil, errors.Wrapf(err, "failed to load secret: %s", meta.Name)
	}

	return &icinga.Config{
		BasicAuth: struct {
			Username string
			Password string
		}{
			Username: string(secret.Data[icinga.ICINGA_API_USER]),
			Password: string(secret.Data[icinga.ICINGA_API_PASSWORD]),
		},
	}, nil
}
