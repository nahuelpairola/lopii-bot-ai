package orchestrator

// Las reglas de clasificación de movimientos que comparten, verbatim, los dos
// caminos de CREATE: createSystemPromptTemplate (el camino pre-loop, que sigue
// vivo detrás de routeCreateToLoop) y agentSystemPromptTemplate (el loop).
//
// Estaban duplicadas. El 2026-08-08 el fix de las piernas del transfer tuvo que
// pegar el mismo párrafo en los dos archivos; tocar uno solo dejaba el camino
// pre-loop clasificando mal, y nada avisaba.
//
// Son DOS consts y no una porque REGLA DE FECHA se mete en el medio y ahí los
// dos prompts difieren de verdad: el loop aclara la zona horaria y agrega la
// resolución de rangos para las herramientas de consulta, que create no tiene.
// Esa regla queda duplicada A PROPÓSITO — no es el mismo texto, es texto
// parecido, y unificarlo obligaría a meterle a create una instrucción sobre una
// herramienta que no está en su toolbox.
//
// MISMA restricción que numberFormatRule: ni '%' literal (se concatenan en
// plantillas de fmt.Sprintf; el '%%' de abajo es el escape correcto) ni
// backtick (son raw string literals).

const taxonomyAndAmountRules = `
REGLAS DE TAXONOMÍA:
1. Usá ÚNICAMENTE las categorías y subcategorías listadas abajo. Prohibido inventar nombres nuevos.
2. Si tenés menos del 90%% de certeza sobre la categoría o subcategoría, asigná EXACTAMENTE "PENDING_REVIEW" en ambos campos.

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
- Si el destino de la plata es una de las CUENTAS DEL USUARIO (abajo) → es una transferencia entre cuentas propias (2 transfers, mismo group). Si el destino NO está en esa lista (una persona, un comercio) → es un expense; ese nombre externo va en merchant, nunca como cuenta.
- Si el origen de la plata NO es una cuenta tuya (alguien te mandó plata) → income.
- El verbo (transferí, pasé, di, mandé, pagué) NO decide el tipo; solo sugiere payment_method.
- Los montos de expense/income van en POSITIVO; la app les pone el signo. Solo las piernas de transfer/compra-venta USD llevan un monto negativo explícito.

REGLA DE GANANCIA:
- Una cuenta que rinde ("el plazo fijo rindió 5000", "el broker ganó 10 mil") es un income en esa cuenta, subcategoría "Sistema | Rendimiento inversión". Nunca un income genérico, nunca un transfer.

REGLA DE AGRUPACIÓN (campo group):
- Las piernas/ítems de UNA operación atómica (compra/venta USD, transferencia entre cuentas propias, suscripción/rescate FCI) llevan el MISMO group. Compras u operaciones separadas — incluso ítems de una tarjeta ("pan, medicamentos, carne"; "ropa, super") — NO llevan group.
`
