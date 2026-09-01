package orchestrator

import "strings"

const amountRules = `
REGLAS DE MONTO Y MONEDA:
- Los montos abreviados ("200k", "1.5m") se expanden a su valor numérico completo.
- Si el mensaje no aclara moneda, asumí ARS siempre.
- Método de pago por defecto si no se aclara: "transfer". Vocabulario sugerido: transfer, qr, cash, bank_deposit, check, broker, credit_card.
` + numberFormatRule

const movementPatternRules = `
PATRONES DE MOVIMIENTOS COMPUESTOS (varios movimientos, comparten una misma transacción):
1. Compra/venta de USD ("compré 100 usd a 1500", "vendí 100 usd a 1400"): 2 transfers en una misma transacción, AMBOS con account_id, subcategoría "Inversiones | Dólares". Nunca expense/income. Compra: uno negativo en ARS saliendo de la cuenta ARS origen (por el total en pesos, ej. -150000) y uno positivo en USD entrando a la cuenta USD destino (ej. +100). Venta: espejo (negativo en USD saliendo de la cuenta USD, positivo en ARS entrando a la cuenta ARS). Si no se aclara la cuenta de una pierna, usá la cuenta por defecto de esa moneda.
2. Transferencia entre cuentas propias ("pasé 50 mil del banco a Mercado Pago"): 2 transfers en una misma transacción — uno negativo saliendo de la cuenta origen, uno positivo entrando a la cuenta destino, MISMA moneda en ambas piernas, subcategoría "Sistema | Transferencia". Nunca expense/income. Ambas piernas llevan account_id. Si las monedas difieren, NO es esto: es una compra/venta de USD (patrón 1). CONTROL OBLIGATORIO: los amount de las dos piernas tienen que sumar EXACTAMENTE 0 (ej. "pasé 200 mil de Galicia a Mercado Pago" → amount "-200000" en Galicia y "200000" en Mercado Pago). Si te da el doble del monto, pusiste las dos en positivo: corregilo ANTES de responder.
3. Suscripción a FCI ("suscribí a FCI 120k"): 2 transfers — afuera de la cuenta origen, adentro de la cuenta FCI. Nunca expense/income.
4. Rescate de FCI ("rescaté 100k de FCI"): 2 transfers — afuera de la cuenta FCI, adentro del destino. NO calcules ni agregues ganancia vos: nunca devuelvas un movimiento de income por un rescate, eso lo calcula la aplicación.
5. Itemización de tarjeta ("pago tarjeta 200k: ropa 50k, super 150k"): un expense por cada ítem nombrado, misma transacción. Si el mensaje aclara un total y dice que "el resto" es algo (ej. cargos de tarjeta), calculá ese resto vos mismo (total declarado menos la suma de los ítems nombrados) y agregalo como un expense más. Si la suma de los ítems nombrados supera el total declarado, no devuelvas ningún movimiento — es un error de datos del usuario.

REGLA DE TIPO (el destino decide el tipo, el verbo NO):
- Si el destino de la plata es una de las CUENTAS DEL USUARIO (abajo) → es una transferencia entre cuentas propias (2 transfers, mismo group). Si el destino NO está en esa lista (una persona, un comercio) → es un expense; ese nombre externo es parte de QUÉ pasó y va en description, nunca como cuenta. account_name_guess es SOLO para cuentas, bancos y billeteras.
- Si el origen de la plata NO es una cuenta tuya (alguien te mandó plata) → income.
- El verbo (transferí, pasé, di, mandé, pagué) NO decide el tipo; solo sugiere payment_method.
- Los montos de expense/income van en POSITIVO; la app les pone el signo. Solo las piernas de transfer/compra-venta USD llevan un monto negativo explícito.

REGLA DE GANANCIA:
- Una cuenta que rinde ("el plazo fijo rindió 5000", "el broker ganó 10 mil") es un income en esa cuenta, subcategoría "Sistema | Rendimiento inversión". Nunca un income genérico, nunca un transfer.

REGLA DE AGRUPACIÓN (campo group):
- Las piernas/ítems de UNA operación atómica (compra/venta USD, transferencia entre cuentas propias, suscripción/rescate FCI) llevan el MISMO group. Compras u operaciones separadas — incluso ítems de una tarjeta ("pan, medicamentos, carne"; "ropa, super") — NO llevan group.
`

var agentPatternRules = strings.NewReplacer(
	`, subcategoría "Inversiones | Dólares"`, "",
	`, subcategoría "Sistema | Transferencia"`, "",
	rendimientoRule, "",
).Replace(movementPatternRules)

const rendimientoRule = `
REGLA DE GANANCIA:
- Una cuenta que rinde ("el plazo fijo rindió 5000", "el broker ganó 10 mil") es un income en esa cuenta, subcategoría "Sistema | Rendimiento inversión". Nunca un income genérico, nunca un transfer.
`
