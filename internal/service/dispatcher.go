package service

import (
	"context"
	"fmt"
	"log"
	"math/rand/v2"
	"sync"
	"time"

	"api/internal/domain"
)

const (
	channelWhatsApp = "whatsapp"
	channelEmail    = "email"

	// queueCapacity é folgado de propósito: enfileirar não pode bloquear o
	// handler HTTP que acabou de fazer o match.
	queueCapacity = 256

	// sendTimeout limita um envio individual. Generoso porque inclui upload de
	// mídia no WhatsApp e refresh de token OAuth no e-mail.
	sendTimeout = 2 * time.Minute
)

// dispatchJob é uma candidatura a ser enviada.
type dispatchJob struct {
	Channel     string
	Target      string
	VacancyID   string
	SenderPhone string
	Subject     string
	Body        string
	FileB64     string
	Filename    string

	// LogFile é o arquivo de execução do match que gerou este job. O desfecho do
	// envio é anexado nele — é o único registro que sobrevive a um restart.
	LogFile string

	// EmailService é o cliente OAuth do candidato, só para jobs de e-mail. Viaja
	// no job porque o envio acontece muito depois da requisição que o criou, e
	// rebuscar a credencial na hora seria um refresh de token por disparo.
	EmailService domain.EmailService

	// onResult recebe o status final do envio. O orchestrator usa isso para
	// atualizar o Match no histórico, já devolvido ao cliente HTTP.
	onResult func(status string)
}

// dispatcher enfileira candidaturas por canal e as envia espaçadas no tempo.
//
// Uma fila por canal, drenadas em paralelo: o limite que se quer respeitar é por
// canal, então um e-mail e um WhatsApp podem sair ao mesmo tempo sem risco.
//
// ponytail: a fila vive só em memória. Um restart no meio descarta os envios
// pendentes, e cada um é uma candidatura que a pessoa perdeu. Aceito em troca de
// não gravar o PDF do currículo no banco a cada job. Upgrade path: tabela
// dispatch_queue no api.db, com status e retry.
type dispatcher struct {
	queues map[string]chan dispatchJob
	send   func(ctx context.Context, job dispatchJob) error

	// delay é injetável para os testes não esperarem 20 segundos de verdade.
	delay func() time.Duration

	wg sync.WaitGroup
}

// newDispatcher sobe um worker por canal. send faz o envio de fato; delay pode
// ser nil, caso em que usa o espaçamento de produção.
func newDispatcher(send func(ctx context.Context, job dispatchJob) error, delay func() time.Duration) *dispatcher {
	if delay == nil {
		delay = sendDelay
	}

	d := &dispatcher{
		queues: make(map[string]chan dispatchJob, 2),
		send:   send,
		delay:  delay,
	}

	for _, channel := range []string{channelWhatsApp, channelEmail} {
		queue := make(chan dispatchJob, queueCapacity)
		d.queues[channel] = queue
		d.wg.Add(1)
		go d.worker(channel, queue)
	}

	return d
}

// sendDelay devolve o intervalo entre dois envios do mesmo canal: 20 a 40
// segundos. O objetivo é não parecer um robô disparando em rajada, que é o que o
// WhatsApp pune com banimento.
func sendDelay() time.Duration {
	return 20*time.Second + rand.N(20001*time.Millisecond)
}

// enqueue põe job na fila do seu canal. Retorna erro se o canal for
// desconhecido ou se a fila estiver cheia — nos dois casos sem bloquear o
// chamador, que é uma requisição HTTP esperando resposta.
func (d *dispatcher) enqueue(job dispatchJob) error {
	queue, ok := d.queues[job.Channel]
	if !ok {
		return fmt.Errorf("canal de envio desconhecido: %q", job.Channel)
	}

	select {
	case queue <- job:
		return nil
	default:
		return fmt.Errorf("fila de %s cheia (%d itens), envio descartado", job.Channel, queueCapacity)
	}
}

// worker drena uma fila, espaçando os envios. O primeiro job sai na hora; o
// atraso vem antes de cada envio seguinte, que é onde mora o risco de parecer
// rajada.
func (d *dispatcher) worker(channel string, jobs <-chan dispatchJob) {
	defer d.wg.Done()

	first := true
	for job := range jobs {
		if !first {
			wait := d.delay()
			log.Printf("[Dispatcher] Aguardando %s antes do próximo envio via %s...", wait.Round(time.Second), channel)
			time.Sleep(wait)
		}
		first = false

		// context.Background() de propósito: o envio não pode morrer porque a
		// requisição que o enfileirou já terminou — é justamente o ponto da fila.
		ctx, cancel := context.WithTimeout(context.Background(), sendTimeout)
		err := d.send(ctx, job)
		cancel()

		status := "Sucesso (enviado)"
		if err != nil {
			status = fmt.Sprintf("Erro no envio: %v", err)
			log.Printf("[Dispatcher] Falha no envio via %s para %s (vaga %s): %v", channel, job.Target, job.VacancyID, err)
		} else {
			log.Printf("[Dispatcher] Enviado via %s para %s (vaga %s)", channel, job.Target, job.VacancyID)
		}

		appendDispatchOutcome(job.LogFile, channel, job.Target, job.VacancyID, status)

		if job.onResult != nil {
			job.onResult(status)
		}
	}
}

// stop fecha as filas e espera os workers terminarem o que já pegaram. Os jobs
// ainda enfileirados são drenados normalmente, então isso pode demorar.
func (d *dispatcher) stop() {
	for _, queue := range d.queues {
		close(queue)
	}
	d.wg.Wait()
}
