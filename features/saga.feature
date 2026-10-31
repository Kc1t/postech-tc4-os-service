# language: pt
Funcionalidade: Saga da ordem de serviço
  Para que o cliente pague só pelo que foi feito e nunca fique com estoque ou cobrança pendurados
  Como oficina
  Eu quero que diagnóstico, orçamento, pagamento e reparo avancem juntos ou sejam desfeitos juntos

  Os serviços billing e execution são representados pelas mensagens que eles trocam com o
  orquestrador (os comandos saem pelo outbox, as respostas entram pela fila os-saga-replies).

  Contexto:
    Dado um cliente com um veículo cadastrado
    E uma peça com 5 unidades em estoque

  Cenário: caminho feliz até a entrega do veículo
    Quando a equipe abre uma ordem de serviço
    Então o execution-service recebe o comando "execution.StartDiagnosis"
    Quando o mecânico conclui o diagnóstico pedindo 2 unidades da peça
    Então o estoque da peça fica em 3
    E o billing-service recebe o comando "billing.CreateBudget" somando R$ 330.00
    Quando o billing-service confirma o orçamento
    E o cliente aprova o orçamento
    Então o billing-service recebe o comando "billing.RequestPayment"
    Quando o billing-service gera o link de pagamento
    E o pagamento é aprovado
    Então o execution-service recebe o comando "execution.StartRepair"
    Quando o mecânico inicia e finaliza o reparo
    E a equipe entrega o veículo
    Então a ordem de serviço fica "delivered"
    E a saga fica "completed"
    E o histórico da ordem é "received, in_diagnosis, awaiting_budget, awaiting_approval, awaiting_payment, in_execution, finished, delivered"
    E o cliente é notificado 8 vezes

  Cenário: cliente recusa o orçamento
    Dado uma ordem de serviço com o orçamento aguardando aprovação
    Quando o cliente recusa o orçamento
    Então a ordem de serviço fica "cancelled"
    E o estoque da peça fica em 5
    E o execution-service recebe o comando "execution.CancelExecution"
    E o billing-service não recebe o comando "billing.CancelBudget"

  Cenário: pagamento recusado desfaz a reserva e o orçamento
    Dado uma ordem de serviço com o orçamento aguardando aprovação
    Quando o cliente aprova o orçamento
    E o pagamento é recusado
    Então a ordem de serviço fica "cancelled"
    E a saga fica "cancelled"
    E o estoque da peça fica em 5
    E o billing-service recebe o comando "billing.CancelBudget"
    E o execution-service recebe o comando "execution.CancelExecution"
    E o billing-service não recebe o comando "billing.RefundPayment"

  Cenário: reparo recusado depois do pagamento gera estorno
    Dado uma ordem de serviço com o orçamento aguardando aprovação
    Quando o cliente aprova o orçamento
    E o pagamento é aprovado
    E o execution-service recusa o reparo
    Então o billing-service recebe o comando "billing.RefundPayment"
    E a saga fica "compensating"
    E o estoque da peça fica em 5
    Quando o billing-service confirma o estorno
    Então a ordem de serviço fica "cancelled"
    E a saga fica "cancelled"

  Cenário: prazo de aprovação vence sem resposta do cliente
    Dado uma ordem de serviço com o orçamento aguardando aprovação
    Quando passam 73 horas sem resposta do cliente
    E o verificador de prazos roda
    Então a ordem de serviço fica "cancelled"
    E o estoque da peça fica em 5
    E o billing-service recebe o comando "billing.CancelBudget"

  Cenário: mensagem reentregue pela fila não é processada duas vezes
    Quando a equipe abre uma ordem de serviço
    E o mecânico conclui o diagnóstico pedindo 1 unidades da peça
    E a mesma confirmação de diagnóstico chega de novo
    Então o estoque da peça fica em 4
    E o billing-service recebe 1 vez o comando "billing.CreateBudget"

  Esquema do Cenário: falha simulada na etapa <etapa> sempre termina compensada
    Quando a equipe abre uma ordem de serviço simulando falha em "<etapa>"
    E a saga percorre o fluxo até a etapa com falha
    Então a ordem de serviço fica "cancelled"
    E o estoque da peça fica em 5
    E o billing-service recebe <estornos> vez o comando "billing.RefundPayment"

    Exemplos:
      | etapa   | estornos |
      | budget  | 0        |
      | payment | 0        |
      | repair  | 1        |
