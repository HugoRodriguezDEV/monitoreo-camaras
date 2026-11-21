package alertas

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/HugoRodriguezDEV/monitoreo-camaras/internal/config"
)

func Enviar(ctx context.Context, telegram config.Telegram, mensaje string) error {
	if telegram.Token == "" {
		return nil
	}
	datos := url.Values{"chat_id": {telegram.ChatID}, "text": {mensaje}}
	endpoint := "https://api.telegram.org/bot" + telegram.Token + "/sendMessage"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(datos.Encode()))
	if err != nil {
		return fmt.Errorf("no se pudo preparar la alerta de Telegram")
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	cliente := http.Client{
		Timeout:       5 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	res, err := cliente.Do(req)
	if err != nil {
		return fmt.Errorf("no se pudo contactar a Telegram")
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("Telegram devolvió HTTP %d", res.StatusCode)
	}
	_, _ = io.Copy(io.Discard, io.LimitReader(res.Body, 64<<10))
	return nil
}
