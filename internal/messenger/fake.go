package messenger

import (
	"context"

	"lopiibot.com/internal/conversation"
)

// FakeChat es el Chat de los tests. Vive en el paquete y no en un _test.go
// porque lo usan los tests de otros nueve paquetes.
//
// Reemplaza al `if b == nil { return }` que guardaba cada envío: los tests ya
// no pasan un bot nulo, pasan esto y pueden AFIRMAR qué se mandó.
type FakeChat struct {
	Sent  []conversation.Prompt
	Typed int
	Err   error
}

func (f *FakeChat) Send(_ context.Context, p conversation.Prompt) error {
	if f.Err != nil {
		return f.Err
	}
	f.Sent = append(f.Sent, p)
	return nil
}

// Typing cuenta cada llamada en Typed, para que un test pueda afirmar que se
// mostró "escribiendo..." sin inspeccionar el mensaje mandado. Simétrico con
// Send: si Err está seteado, la llamada "no pasó" y no se cuenta.
func (f *FakeChat) Typing(context.Context) error {
	if f.Err != nil {
		return f.Err
	}
	f.Typed++
	return nil
}

// LastText devuelve el texto del último prompt mandado, o "" si no hubo.
func (f *FakeChat) LastText() string {
	if len(f.Sent) == 0 {
		return ""
	}
	return f.Sent[len(f.Sent)-1].Text
}
