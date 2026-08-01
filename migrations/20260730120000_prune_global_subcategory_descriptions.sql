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
-- NO re-correr 20260710130000_reseed_global_subcategories.sql para revertir
-- esto. Ese reseed empieza con:
--
--   UPDATE subcategories SET deleted_at = NOW(), is_global = FALSE WHERE is_global = TRUE;
--
-- o sea que soft-deletea las globales ACTUALES e inserta otras con IDs nuevos.
-- Los movimientos ya cargados apuntan a los IDs viejos, y GORM excluye las filas
-- con deleted_at de toda consulta: los movimientos de los usuarios
-- desaparecerían de la vista. No es una reversión, es una pérdida.
--
-- La reversión real es devolver el texto donde estaba, sin tocar una sola fila
-- de más.
UPDATE subcategories SET description = 'Salario mensual en relación de dependencia.' WHERE is_global AND deleted_at IS NULL AND subcategory = 'Sueldo';
UPDATE subcategories SET description = 'Pago mensual de alquiler de vivienda.' WHERE is_global AND deleted_at IS NULL AND subcategory = 'Alquiler';
UPDATE subcategories SET description = 'Expensas de edificio o barrio cerrado.' WHERE is_global AND deleted_at IS NULL AND subcategory = 'Expensas';
UPDATE subcategories SET description = 'Seguro de vivienda o su contenido.' WHERE is_global AND deleted_at IS NULL AND subcategory = 'Seguro hogar';
UPDATE subcategories SET description = 'Seguro de auto, moto u otro vehículo.' WHERE is_global AND deleted_at IS NULL AND subcategory = 'Seguro vehículo';
UPDATE subcategories SET description = 'Peajes de autopistas o rutas.' WHERE is_global AND deleted_at IS NULL AND subcategory = 'Peaje';
UPDATE subcategories SET description = 'Consultas con clínicos o especialistas.' WHERE is_global AND deleted_at IS NULL AND subcategory = 'Consulta médica';
UPDATE subcategories SET description = 'Laboratorio, radiografías o estudios clínicos.' WHERE is_global AND deleted_at IS NULL AND subcategory = 'Estudios / análisis';
UPDATE subcategories SET description = 'Cuota mensual de cobertura médica.' WHERE is_global AND deleted_at IS NULL AND subcategory = 'Obra social / prepaga';
UPDATE subcategories SET description = 'Dentista y tratamientos dentales.' WHERE is_global AND deleted_at IS NULL AND subcategory = 'Odontología';
UPDATE subcategories SET description = 'Sesiones de psicólogo o terapia.' WHERE is_global AND deleted_at IS NULL AND subcategory = 'Psicología';
UPDATE subcategories SET description = 'Cine, teatro, recitales, shows o entradas a eventos.' WHERE is_global AND deleted_at IS NULL AND subcategory = 'Entretenimiento';
UPDATE subcategories SET description = 'Vuelos, hoteles, excursiones y gastos de viaje.' WHERE is_global AND deleted_at IS NULL AND subcategory = 'Viajes / vacaciones';
UPDATE subcategories SET description = 'Cuota mensual de gimnasio o club.' WHERE is_global AND deleted_at IS NULL AND subcategory = 'Gimnasio';
UPDATE subcategories SET description = 'Carteras, cinturones, mochilas, relojes o joyería.' WHERE is_global AND deleted_at IS NULL AND subcategory = 'Accesorios';
UPDATE subcategories SET description = 'Celular, notebook, PC, tablet o electrodomésticos electrónicos.' WHERE is_global AND deleted_at IS NULL AND subcategory = 'Dispositivos';
UPDATE subcategories SET description = 'Libros físicos o digitales, apuntes, fotocopias o material de estudio.' WHERE is_global AND deleted_at IS NULL AND subcategory = 'Libros / materiales';
UPDATE subcategories SET description = 'Consultas, vacunas o tratamientos veterinarios.' WHERE is_global AND deleted_at IS NULL AND subcategory = 'Veterinaria';
UPDATE subcategories SET description = 'Cuota de un crédito o préstamo (personal, prendario, hipotecario).' WHERE is_global AND deleted_at IS NULL AND subcategory = 'Cuota préstamo';
UPDATE subcategories SET description = 'Compra o venta de criptomonedas.' WHERE is_global AND deleted_at IS NULL AND subcategory = 'Cripto';
UPDATE subcategories SET description = 'Compra o venta de dólares (físicos o digitales).' WHERE is_global AND deleted_at IS NULL AND subcategory = 'Dólares';
UPDATE subcategories SET description = 'Otras inversiones no clasificadas.' WHERE is_global AND deleted_at IS NULL AND subcategory = 'Otros activos';
UPDATE subcategories SET description = 'Constitución o cobro de plazo fijo.' WHERE is_global AND deleted_at IS NULL AND subcategory = 'Plazo fijo';
UPDATE subcategories SET description = 'Mantenimiento de cuenta, comisiones bancarias o de broker.' WHERE is_global AND deleted_at IS NULL AND subcategory = 'Cargos / comisiones bancarias';
UPDATE subcategories SET description = 'Regalos a terceros y donaciones.' WHERE is_global AND deleted_at IS NULL AND subcategory = 'Regalos / donaciones';
UPDATE subcategories SET description = 'Movimiento de apertura de cuenta.' WHERE is_global AND deleted_at IS NULL AND subcategory = 'Saldo inicial';
