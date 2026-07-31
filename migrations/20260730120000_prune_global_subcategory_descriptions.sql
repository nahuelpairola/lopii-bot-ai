-- +goose Up
-- El bloque de taxonomía va inline en el prompt de CREATE en cada llamada: 64
-- filas. La mayoría de las descripciones repiten el nombre de la subcategoría
-- ("Sueldo" → "Salario mensual en relación de dependencia"); solo las que
-- contrastan con una hermana o mapean una palabra/comercio argentino que el
-- nombre no dice ("SUBE", "Edenor", "remis", "ABL") cambian lo que el modelo
-- elige. Se vacían las que no deciden nada.
--
-- buildTaxonomyBlock omite el tercer campo cuando la descripción está vacía, así
-- que una fila podada renderiza como "Categoría | Subcategoría".
--
-- Se vacía con '' y no con NULL: subcategory.Subcategory.Description es un string
-- de Go, no un puntero.
--
-- Acotado a is_global = TRUE: las subcategorías propias de un usuario llevan
-- descripciones que él escribió o confirmó, y esas no son nuestras para editar.
--
-- Los nombres de abajo son únicos entre las 65 globales, por eso alcanza un IN
-- plano sin par (categoría, subcategoría).
--
-- QUÉ ESTÁ VERIFICADO. El corte lo decide TestCreatePruningEval, que le pasa al
-- modelo el mismo mensaje con el bloque de antes y el de después y compara el par
-- que elige. De las 26 podadas hay 21 confirmadas sin cambio. Quedan 5 SIN PROBAR
-- porque el eval se quedó sin cuota diaria de Groq a mitad de corpus:
--
--   Cuota préstamo · Cripto · Dólares · Otros activos · Plazo fijo ·
--   Cargos / comisiones bancarias · Regalos / donaciones
--
-- Antes de mergear esto conviene correr el eval de nuevo con cuota fresca. La
-- regla que salió de la corrida: rompe la descripción que aporta una palabra que
-- el nombre NO contiene y que tiene una hermana plausible.
UPDATE subcategories
SET description = ''
WHERE is_global = TRUE
  AND deleted_at IS NULL
  AND subcategory IN (
    -- Ingresos: el nombre ya dice "relación de dependencia".
    -- NO se podan 'Freelance / honorarios' ni 'Reintegros': el eval diferencial
    -- los vio romper ("cobré una changa" → Sueldo, "me reintegraron 20 mil" →
    -- Otros ingresos). "changa" y "reintegro/reembolso" son palabras que el
    -- nombre no contiene y que tienen una hermana plausible.
    'Sueldo',
    -- Vivienda: quedan Luz/Gas/Agua (mapean Edesur, Metrogas, AySA),
    -- Internet / cable, Impuestos inmueble (ABL) y Mantenimiento hogar.
    'Alquiler',
    'Expensas',
    'Seguro hogar',
    -- Transporte: queda Combustible (nafta/GNC), Service / repuestos,
    -- Patente, Estacionamiento, Transporte público (SUBE) y Uber / taxi (remis).
    'Seguro vehículo',
    'Peaje',
    -- Salud: queda solo Medicamentos / farmacia, que decide dónde va higiene
    -- y skincare frente a Bienestar / Cuidado personal.
    -- NO se poda 'Óptica': sin la nota, "compré anteojos" cae en PENDING_REVIEW.
    -- 'Odontología' y 'Psicología' sí se podan: el eval confirmó que "dentista"
    -- y "terapia" llegan igual sin nota.
    'Consulta médica',
    'Estudios / análisis',
    'Obra social / prepaga',
    'Odontología',
    'Psicología',
    -- Ocio y salidas: quedan Salir a comer y Delivery (se contrastan entre sí),
    -- Hobby (separa de Entretenimiento) y Deporte (NO gimnasio).
    'Entretenimiento',
    'Viajes / vacaciones',
    -- Bienestar: el contraste con Deporte vive en la nota de Deporte.
    'Gimnasio',
    -- Indumentaria: la categoría ya lo separa de Tecnología / Accesorios.
    'Accesorios',
    -- Tecnología: el contraste vive en la nota de Accesorios / reparación.
    'Dispositivos',
    -- Educación: queda Cursos / capacitación (Udemy, Platzi, Coursera).
    'Libros / materiales',
    -- Mascotas: queda Alimento / accesorios mascota (peluquería de mascota
    -- frente a Bienestar / Cuidado personal).
    'Veterinaria',
    -- Deudas: queda Cuota / financiación tarjeta, que es la que decide que los
    -- ítems de un resumen NO van acá.
    'Cuota préstamo',
    -- Inversiones: quedan FCI (expande la sigla) y Acciones / CEDEARs (bonos).
    'Cripto',
    'Dólares',
    'Otros activos',
    'Plazo fijo',
    -- Finanzas: quedan Impuestos financieros y Monotributo / AFIP.
    'Cargos / comisiones bancarias',
    -- Otros
    'Regalos / donaciones',
    -- Sistema: quedan Rendimiento inversión y Transferencia ("propias" decide).
    'Saldo inicial'
  );

-- +goose Down
-- Irreversible a propósito: el texto original vive en
-- 20260710130000_reseed_global_subcategories.sql. Re-correr ese reseed lo
-- restaura.
SELECT 1;
