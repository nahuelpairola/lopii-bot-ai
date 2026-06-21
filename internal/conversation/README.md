# Conversation Engine: Flows y Steps

## 🎯 Conceptos Centrales

El sistema ejecuta **conversaciones guiadas** (flujos) con el usuario mediante **pasos** (steps) que acumulan datos:

```
Inicio → Step 1 (pregunta) → Step 2 (procesa input) → Step 3 → Fin
                     ↓                    ↓
                  Data{key1: val}    Data{key1: val, key2: val2}
```

---

## 📊 Componentes

### 1. **Step** (Unidad básica)
Cada step es una pregunta o decisión. Implementa 3 métodos:

```go
type Step interface {
    Prompt(data Data) Prompt                    // ¿Qué muestra al usuario?
    Process(input Input, data Data) Transition  // ¿Cómo procesa su respuesta?
    PossibleNextSteps() []string                // ¿Cuáles son los posibles siguientes?
}
```

**Dos tipos listos para usar:**

#### TextStep (Texto libre)
Pregunta abierta, valida, guarda en una clave:

```go
conversation.TextStep{
    PromptText: "¿Nombre de la cuenta?",
    DataKey:    "account_name",
    Validate: func(text string, data Data) error {
        if len(text) < 3 {
            return errors.New("muy corto")
        }
        return nil
    },
    NextStep: "confirm_name",
}
```

#### ChoiceStep (Botones/Opciones)
Opciones predefinidas, cada una elige un destino:

```go
conversation.ChoiceStep{
    PromptText: "¿Moneda?",
    Options: []conversation.ChoiceOption{
        {Label: "USD", Value: "usd", NextStep: "amount"},
        {Label: "ARS", Value: "ars", NextStep: "amount"},
    },
}
```

---

### 2. **Flow** (Grafo de steps)
Conecta todos los steps en un flujo coherente:

```go
flow, err := conversation.NewFlow(
    "account_setup",           // nombre único
    "choose_currency",         // step inicial
    map[string]conversation.Step{
        "choose_currency": choiceCurrencyStep,
        "account_name":    textNameStep,
        "confirm_name":    confirmStep,
    },
)
```

El Flow **valida que exista cada NextStep referenciado** → falla al inicio, no en runtime.

---

### 3. **Engine** (Orquestador)
Ejecuta flujos, gestiona estado, persiste en DB:

```go
engine := conversation.NewEngine(repo)
engine.Register(flow)

// Usuario inicia flujo
prompt, _ := engine.Start(userID, "account_setup")
// → Retorna Prompt del primer step

// Usuario responde
result, _ := engine.Handle(userID, input)
// → Procesa, guarda estado, retorna próximo Prompt o resultado final
```

---

## 🔄 Ciclo de Vida

```
1. START(userID, flowName)
   ↓
   Engine crea Data{_user_id: userID}
   Engine persiste: (userID, flowName, firstStep, {})
   ↓
   Retorna Prompt del primer step

2. HANDLE(userID, input)
   ↓
   Engine obtiene estado: (flowName, stepName, data)
   Step.Process(input, data) → retorna Transition
   ↓
   Si Transition.kind == "retry":
       Engine mantiene step, retorna Prompt con error
   
   Si Transition.kind == "advance":
       Engine guarda: (userID, flowName, nextStep, newData)
       Retorna Prompt del siguiente step
   
   Si Transition.kind == "complete":
       Engine borra estado
       Retorna resultado final + Data acumulada
```

---

## 💾 Data: Estado Acumulativo

`Data` es un mapa que crece a medida que el usuario avanza:

```
Step 1: Elige USD      → Data{_user_id: 123, currency: "usd"}
Step 2: Ingresa nombre → Data{_user_id: 123, currency: "usd", name: "Banco"}
Step 3: Confirma       → Data{_user_id: 123, currency: "usd", name: "Banco", ...}
```

**Acceso en Steps:**

```go
func (s myStep) Prompt(data Data) Prompt {
    currency := data["currency"].(string)
    return Prompt{Text: fmt.Sprintf("Para %s, ¿saldo?", currency)}
}

func (s myStep) Process(input Input, data Data) Transition {
    next := copyData(data)
    next["balance"] = parseAmount(input.Text)
    return conversation.Advance("confirm_step", next)
}
```

---

## 🛠️ Ejemplo Completo: Crear una Cuenta

**Pasos:**
1. Elegir moneda (USD/ARS)
2. Ingresar nombre
3. Confirmar si es default
4. (COMENTADO) Ingresar saldo inicial

**Setup:**

```go
func newAccountSetupFlow(repo Repository) (*conversation.Flow, error) {
    steps := map[string]conversation.Step{
        "choose_currency": conversation.ChoiceStep{
            PromptText: "¿Moneda?",
            Options: []conversation.ChoiceOption{
                {Label: "USD", Value: "usd", NextStep: "account_name"},
                {Label: "ARS", Value: "ars", NextStep: "account_name"},
            },
            OnChoice: func(val string, data Data) Data {
                next := copyData(data)
                next["currency"] = val
                return next
            },
        },
        "account_name": conversation.TextStep{
            PromptText: "Nombre de la cuenta:",
            DataKey:    "name",
            Validate: func(text string, _ Data) error {
                if strings.TrimSpace(text) == "" {
                    return errors.New("no puede estar vacío")
                }
                return nil
            },
            NextStep: "confirm_default",
        },
        "confirm_default": conversation.ChoiceStep{
            PromptText: "¿Usar como default?",
            Options: []conversation.ChoiceOption{
                {Label: "Sí", Value: "yes", NextStep: "choose_currency", Finish: false},
                {Label: "No", Value: "no", NextStep: "choose_currency", Finish: false},
            },
        },
    }

    return conversation.NewFlow("account_setup", "choose_currency", steps)
}

// En main/server.go:
flow, _ := newAccountSetupFlow(repo)
engine.Register(flow)
```

---

## 🔁 Reutilización: Editar una Cuenta

Mismo patrón, pero con datos pre-cargados:

```go
func newAccountEditFlow(repo Repository) (*conversation.Flow, error) {
    steps := map[string]conversation.Step{
        // TextStep: editar nombre (igual que antes)
        "edit_name": conversation.TextStep{
            PromptText: func(data Data) string {
                current := data["current_name"].(string)
                return fmt.Sprintf("Nombre actual: %s\nNuevo nombre:", current)
            },
            DataKey:   "name",
            Validate:  /* igual */,
            NextStep:  "confirm_edit",
        },
        
        "confirm_edit": conversation.ChoiceStep{
            PromptText: "¿Guardar cambios?",
            Options: []conversation.ChoiceOption{
                {Label: "Guardar", Value: "yes", Finish: true},
                {Label: "Cancelar", Value: "no", Finish: true},
            },
        },
    }
    
    return conversation.NewFlow("account_edit", "edit_name", steps)
}
```

Antes de iniciar, pre-llena Data con `current_name`:

```go
// En el adaptador (messaging handler):
data := conversation.Data{
    conversation.UserIDKey: userID,
    "current_name":         account.Name,  // ← pre-cargado
}
engine.Start(userID, "account_edit")  // El engine carga esto del estado
```

---

## ⚡ Simplificar: Evitar Complejidad

### ❌ Evita:
- **OnChoice solo para guardar en Data** → Usa `DataKey` en TextStep en su lugar
- **Steps condicionales complejos** → Separa en flujos distintos si es muy ramificado
- **Validaciones fuera del Step** → Implementa `Validate` dentro del TextStep, no después

### ✅ Prefiere:
- **Un step por pregunta clara** → Más pasos simples es mejor que uno complejo
- **NextStep siempre fijo** (o pocas opciones en ChoiceStep)
- **Data mínima** → Solo lo que necesitas en el siguiente step

---

## 📋 Checklist para Crear un Nuevo Flow

1. **Mapea los pasos**: Dibuja el flujo (moneda → nombre → confirmar → fin)
2. **Elige tipo de step**: ¿Texto libre (TextStep) u opciones (ChoiceStep)?
3. **Define Data keys**: Qué información acumulas (currency, name, etc)
4. **Implementa cada step** con `Prompt()`, `Process()`, `PossibleNextSteps()`
5. **Crea el Flow** con `NewFlow()`
6. **Registra en Engine** con `engine.Register()`
7. **Adapta el usuario** (messaging, HTTP) para llamar `Start()` y `Handle()`

---

## 🐛 Debugging

```go
// Ver estado actual de un usuario
flowName, stepName, data, _, err := engine.InProgress(userID)
log.Printf("User %d: Flow=%s, Step=%s, Data=%+v", userID, flowName, stepName, data)

// Limpiar conversación bloqueada
engine.Clear(userID)
```

---

## 📚 Archivos del Módulo

- **engine.go** — Motor de ejecución (Start, Handle, Register)
- **flow.go** — Definición de Flow y tipos base (Step, Data, Transition)
- **text_step.go** — TextStep listo para usar
- **choice_step.go** — ChoiceStep listo para usar
- **repository.go** — Persistencia en Postgres
