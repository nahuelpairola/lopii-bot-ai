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
-- Los 29 nombres de abajo son únicos entre las 64 globales, por eso alcanza un IN
-- plano sin par (categoría, subcategoría).
UPDATE subcategories
SET description = ''
WHERE is_global = TRUE
  AND deleted_at IS NULL
  AND subcategory IN (
    -- Ingresos: los nombres ya separan dependencia de independiente.
    'Sueldo',
    'Freelance / honorarios',
    'Reintegros',
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
    'Consulta médica',
    'Estudios / análisis',
    'Obra social / prepaga',
    'Odontología',
    'Óptica',
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
