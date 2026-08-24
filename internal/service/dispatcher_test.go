package service

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// fixedDelay devolve um atraso curto e previsível, para o teste não esperar os
// 20 a 40 segundos de produção.
func fixedDelay(d time.Duration) func() time.Duration {
	return func() time.Duration { return d }
}

func TestDispatcherSpacesSendsWithinAChannel(t *testing.T) {
	const gap = 40 * time.Millisecond

	var mu sync.Mutex
	var times []time.Time

	d := newDispatcher(func(ctx context.Context, job dispatchJob) error {
		mu.Lock()
		times = append(times, time.Now())
		mu.Unlock()
		return nil
	}, fixedDelay(gap))

	for i := 0; i < 3; i++ {
		if err := d.enqueue(dispatchJob{Channel: channelWhatsApp, Target: "555"}); err != nil {
			t.Fatalf("unexpected enqueue error: %v", err)
		}
	}
	d.stop()

	if len(times) != 3 {
		t.Fatalf("esperava 3 envios, got %d", len(times))
	}
	// O primeiro sai na hora; do segundo em diante tem que haver espaçamento.
	for i := 1; i < len(times); i++ {
		if elapsed := times[i].Sub(times[i-1]); elapsed < gap {
			t.Errorf("envio %d saiu %s após o anterior, esperava ao menos %s", i, elapsed, gap)
		}
	}
}

func TestDispatcherChannelsDoNotBlockEachOther(t *testing.T) {
	// WhatsApp com atraso longo, e-mail sem atraso: se as filas fossem a mesma, o
	// e-mail ficaria preso atrás do WhatsApp.
	emailDone := make(chan struct{})

	d := newDispatcher(func(ctx context.Context, job dispatchJob) error {
		if job.Channel == channelEmail {
			close(emailDone)
		}
		return nil
	}, fixedDelay(0))

	// Enche a fila do WhatsApp primeiro.
	for i := 0; i < 3; i++ {
		if err := d.enqueue(dispatchJob{Channel: channelWhatsApp}); err != nil {
			t.Fatalf("unexpected enqueue error: %v", err)
		}
	}
	if err := d.enqueue(dispatchJob{Channel: channelEmail}); err != nil {
		t.Fatalf("unexpected enqueue error: %v", err)
	}

	select {
	case <-emailDone:
	case <-time.After(2 * time.Second):
		t.Fatal("e-mail não saiu: as filas dos dois canais não são independentes")
	}
	d.stop()
}

func TestDispatcherReportsResultAndSurvivesFailures(t *testing.T) {
	var mu sync.Mutex
	var statuses []string
	record := func(s string) {
		mu.Lock()
		statuses = append(statuses, s)
		mu.Unlock()
	}

	attempt := 0
	d := newDispatcher(func(ctx context.Context, job dispatchJob) error {
		attempt++
		if attempt == 1 {
			return errors.New("recrutador recusou")
		}
		return nil
	}, fixedDelay(0))

	for i := 0; i < 2; i++ {
		if err := d.enqueue(dispatchJob{Channel: channelEmail, onResult: record}); err != nil {
			t.Fatalf("unexpected enqueue error: %v", err)
		}
	}
	d.stop()

	if len(statuses) != 2 {
		t.Fatalf("onResult deve ser chamado para todo job, got %d", len(statuses))
	}
	// Uma falha não pode derrubar o worker nem impedir o job seguinte.
	if statuses[0] != "Erro no envio: recrutador recusou" {
		t.Errorf("primeiro job deve reportar o erro, got %q", statuses[0])
	}
	if statuses[1] != "Sucesso (enviado)" {
		t.Errorf("job após uma falha deve seguir normalmente, got %q", statuses[1])
	}
}

func TestDispatcherRejectsUnknownChannel(t *testing.T) {
	d := newDispatcher(func(ctx context.Context, job dispatchJob) error { return nil }, fixedDelay(0))
	defer d.stop()

	if err := d.enqueue(dispatchJob{Channel: "pombo-correio"}); err == nil {
		t.Error("esperava erro para canal desconhecido")
	}
}

func TestSendDelayStaysInRange(t *testing.T) {
	for i := 0; i < 200; i++ {
		d := sendDelay()
		if d < 20*time.Second || d > 40*time.Second {
			t.Fatalf("sendDelay saiu da faixa de 20-40s: %s", d)
		}
	}
}

// O desfecho do envio precisa ir para o arquivo de execução. Sem isso o log
// congela em "Enfileirado" e o resultado real só existe no stdout e no histórico
// em memória — os dois somem num restart, deixando o operador sem saber se a
// candidatura saiu.
func TestDispatcherAppendsOutcomeToExecutionLog(t *testing.T) {
	logFile := filepath.Join(t.TempDir(), "log_execucao.txt")

	failing := errors.New("recrutador indisponível")
	attempt := 0
	d := newDispatcher(func(ctx context.Context, job dispatchJob) error {
		attempt++
		if attempt == 2 {
			return failing
		}
		return nil
	}, fixedDelay(0))

	jobs := []dispatchJob{
		{Channel: channelEmail, Target: "rh@empresa.com", VacancyID: "vac_ok", LogFile: logFile},
		{Channel: channelEmail, Target: "outro@empresa.com", VacancyID: "vac_falha", LogFile: logFile},
	}
	for _, j := range jobs {
		if err := d.enqueue(j); err != nil {
			t.Fatalf("unexpected enqueue error: %v", err)
		}
	}
	d.stop()

	raw, err := os.ReadFile(logFile)
	if err != nil {
		t.Fatalf("log de execução não foi criado: %v", err)
	}
	content := string(raw)

	for _, want := range []string{
		"vac_ok", "rh@empresa.com", "Sucesso (enviado)",
		"vac_falha", "outro@empresa.com", failing.Error(),
	} {
		if !strings.Contains(content, want) {
			t.Errorf("log deve conter %q, got:\n%s", want, content)
		}
	}
}

// Sem LogFile o dispatcher não pode explodir nem criar arquivo à toa.
func TestDispatcherWithoutLogFileIsSilent(t *testing.T) {
	done := make(chan struct{})
	d := newDispatcher(func(ctx context.Context, job dispatchJob) error {
		close(done)
		return nil
	}, fixedDelay(0))

	if err := d.enqueue(dispatchJob{Channel: channelEmail}); err != nil {
		t.Fatalf("unexpected enqueue error: %v", err)
	}
	<-done
	d.stop()
}
