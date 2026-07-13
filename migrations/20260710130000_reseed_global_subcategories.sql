-- +goose Up
-- Retires the old global taxonomy (soft-delete, not hard-delete — local
-- dev/test movements still reference these rows via FK). Also flips
-- is_global to FALSE so the retired rows vacate subcategories_global_unique_idx
-- (a partial unique index with no deleted_at clause), letting the new
-- inserts below reuse identical reserved names (Sistema | Saldo inicial, etc.)
-- without a conflict. GORM's gorm.Model DeletedAt already auto-excludes
-- deleted_at-set rows from every app query, so this alone makes them
-- invisible to Cache/FindAllForUser/Preload without touching movements.
UPDATE subcategories SET deleted_at = NOW(), is_global = FALSE WHERE is_global = TRUE;

INSERT INTO subcategories (user_id, category, subcategory, description, is_global, icon) VALUES

-- Ingresos
(NULL, 'Ingresos', 'Sueldo',                 'Salario mensual en relación de dependencia.', TRUE, '💵'),
(NULL, 'Ingresos', 'Freelance / honorarios', 'Ingresos por trabajo independiente, changas u honorarios profesionales.', TRUE, '💵'),
(NULL, 'Ingresos', 'Reintegros',             'Devoluciones, reintegros o reembolsos de dinero ya gastado.', TRUE, '💵'),
(NULL, 'Ingresos', 'Otros ingresos',         'Ingresos que no encajan en las anteriores. NO incluye rendimientos de inversión (esos van en Sistema / Rendimiento inversión).', TRUE, '💵'),

-- Vivienda
(NULL, 'Vivienda', 'Alquiler',            'Pago mensual de alquiler de vivienda.', TRUE, '🏠'),
(NULL, 'Vivienda', 'Expensas',            'Expensas de edificio o barrio cerrado.', TRUE, '🏠'),
(NULL, 'Vivienda', 'Luz',                 'Factura de electricidad (Edesur, Edenor). NO gas ni agua.', TRUE, '🏠'),
(NULL, 'Vivienda', 'Gas',                 'Factura de gas (Metrogas, Naturgy). NO luz ni agua.', TRUE, '🏠'),
(NULL, 'Vivienda', 'Agua',                'Factura de agua (AySA u otra). NO luz ni gas.', TRUE, '🏠'),
(NULL, 'Vivienda', 'Internet / cable',    'Internet, cable o telefonía fija (Fibertel, Telecentro, Flow).', TRUE, '🏠'),
(NULL, 'Vivienda', 'Seguro hogar',        'Seguro de vivienda o su contenido.', TRUE, '🏠'),
(NULL, 'Vivienda', 'Impuestos inmueble',  'ABL, inmobiliario o municipal de la vivienda. NO patente de vehículo (Transporte).', TRUE, '🏠'),
(NULL, 'Vivienda', 'Mantenimiento hogar', 'Reparaciones, repuestos, service o mejoras del hogar.', TRUE, '🏠'),

-- Alimentación
(NULL, 'Alimentación', 'Supermercado',    'Compra grande o general en super o hiper (Coto, Carrefour, Disco, Jumbo). NO compra chica de barrio (Almacén / barrio).', TRUE, '🍔'),
(NULL, 'Alimentación', 'Almacén / barrio','Compra chica en almacén, kiosco, dietética, carnicería, verdulería o panadería (el comercio va en merchant). NO compra grande de super.', TRUE, '🍔'),

-- Transporte
(NULL, 'Transporte', 'Combustible',           'Nafta, GNC, diésel o carga eléctrica de cualquier vehículo (el vehículo va en merchant o description).', TRUE, '🚗'),
(NULL, 'Transporte', 'Seguro vehículo',       'Seguro de auto, moto u otro vehículo.', TRUE, '🚗'),
(NULL, 'Transporte', 'Service / repuestos',   'Taller, mantenimiento, service, repuestos o accesorios de vehículo.', TRUE, '🚗'),
(NULL, 'Transporte', 'Patente / impuesto veh.','Patente municipal o provincial de vehículo. NO impuesto de inmueble (Vivienda).', TRUE, '🚗'),
(NULL, 'Transporte', 'Peaje',                 'Peajes de autopistas o rutas.', TRUE, '🚗'),
(NULL, 'Transporte', 'Estacionamiento',       'Playa de estacionamiento, cochera temporal o parquímetro. NO cochera fija mensual de vivienda.', TRUE, '🚗'),
(NULL, 'Transporte', 'Transporte público',    'SUBE, colectivo, tren o subte.', TRUE, '🚗'),
(NULL, 'Transporte', 'Uber / taxi',           'Viajes en Uber, Cabify, DiDi, taxi o remis.', TRUE, '🚗'),

-- Salud
(NULL, 'Salud', 'Consulta médica',        'Consultas con clínicos o especialistas.', TRUE, '🩺'),
(NULL, 'Salud', 'Medicamentos / farmacia','Remedios (con o sin receta) y productos de farmacia (cremas, vitaminas, higiene).', TRUE, '🩺'),
(NULL, 'Salud', 'Obra social / prepaga',  'Cuota mensual de cobertura médica.', TRUE, '🩺'),
(NULL, 'Salud', 'Odontología',            'Dentista y tratamientos dentales.', TRUE, '🩺'),
(NULL, 'Salud', 'Psicología',             'Sesiones de psicólogo o terapia.', TRUE, '🩺'),
(NULL, 'Salud', 'Estudios / análisis',    'Laboratorio, radiografías o estudios clínicos.', TRUE, '🩺'),
(NULL, 'Salud', 'Óptica',                 'Lentes, anteojos o consultas oftalmológicas.', TRUE, '🩺'),

-- Ocio y salidas
(NULL, 'Ocio y salidas', 'Salir a comer',       'Restaurante, bar, café, cervecería o heladería (comer o tomar afuera). NO pedido a domicilio (Delivery).', TRUE, '🎉'),
(NULL, 'Ocio y salidas', 'Delivery',            'Pedidos de comida a domicilio (PedidosYa, Rappi). NO comer en el local (Salir a comer).', TRUE, '🎉'),
(NULL, 'Ocio y salidas', 'Entretenimiento',     'Cine, teatro, recitales, shows o entradas a eventos.', TRUE, '🎉'),
(NULL, 'Ocio y salidas', 'Viajes / vacaciones', 'Vuelos, hoteles, excursiones y gastos de viaje.', TRUE, '🎉'),
(NULL, 'Ocio y salidas', 'Hobby',               'Actividades recreativas personales (gaming, arte, música, juegos).', TRUE, '🎉'),
(NULL, 'Ocio y salidas', 'Deporte',             'Cancha, clases o actividad deportiva. NO cuota de gimnasio (Bienestar).', TRUE, '🎉'),

-- Bienestar
(NULL, 'Bienestar', 'Gimnasio',        'Cuota mensual de gimnasio o club.', TRUE, '🧘'),
(NULL, 'Bienestar', 'Cuidado personal','Peluquería, barbería, estética, depilación o skincare.', TRUE, '🧘'),

-- Indumentaria
(NULL, 'Indumentaria', 'Ropa y calzado','Ropa, zapatillas o zapatos (el rubro va en merchant).', TRUE, '👕'),
(NULL, 'Indumentaria', 'Accesorios',    'Carteras, cinturones, mochilas, relojes o joyería.', TRUE, '👕'),

-- Tecnología
(NULL, 'Tecnología', 'Dispositivos',            'Celular, notebook, PC, tablet o electrodomésticos electrónicos.', TRUE, '💻'),
(NULL, 'Tecnología', 'Accesorios / reparación', 'Periféricos, cables, cargadores, auriculares o reparación de equipos.', TRUE, '💻'),

-- Suscripciones
(NULL, 'Suscripciones', 'Streaming',      'Video y música por suscripción (Netflix, Spotify, Disney+, YouTube Premium).', TRUE, '🔁'),
(NULL, 'Suscripciones', 'Software / apps','Suscripciones a software, apps, nube o storage (Adobe, iCloud, Google One).', TRUE, '🔁'),

-- Educación
(NULL, 'Educación', 'Cursos / capacitación','Cursos, idiomas o capacitación profesional (Udemy, Platzi, Coursera).', TRUE, '📚'),
(NULL, 'Educación', 'Libros / materiales',  'Libros físicos o digitales, apuntes, fotocopias o material de estudio.', TRUE, '📚'),

-- Mascotas
(NULL, 'Mascotas', 'Veterinaria',                  'Consultas, vacunas o tratamientos veterinarios.', TRUE, '🐾'),
(NULL, 'Mascotas', 'Alimento / accesorios mascota','Comida, juguetes, accesorios o peluquería de mascota.', TRUE, '🐾'),

-- Deudas / préstamos
(NULL, 'Deudas / préstamos', 'Cuota préstamo',              'Cuota de un crédito o préstamo (personal, prendario, hipotecario).', TRUE, '💳'),
(NULL, 'Deudas / préstamos', 'Cuota / financiación tarjeta','Cuota de financiación o pago mínimo de tarjeta. NO los ítems de un resumen (esos son gastos sueltos por rubro).', TRUE, '💳'),

-- Inversiones
(NULL, 'Inversiones', 'Dólares',           'Compra o venta de dólares (físicos o digitales).', TRUE, '📈'),
(NULL, 'Inversiones', 'Cripto',            'Compra o venta de criptomonedas.', TRUE, '📈'),
(NULL, 'Inversiones', 'FCI',               'Suscripción o rescate de fondos comunes de inversión.', TRUE, '📈'),
(NULL, 'Inversiones', 'Acciones / CEDEARs','Compra o venta de acciones, CEDEARs o bonos.', TRUE, '📈'),
(NULL, 'Inversiones', 'Plazo fijo',        'Constitución o cobro de plazo fijo.', TRUE, '📈'),
(NULL, 'Inversiones', 'Otros activos',     'Otras inversiones no clasificadas.', TRUE, '📈'),

-- Finanzas
(NULL, 'Finanzas', 'Cargos / comisiones bancarias','Mantenimiento de cuenta, comisiones bancarias o de broker.', TRUE, '🏦'),
(NULL, 'Finanzas', 'Impuestos financieros',        'IVA, impuesto PAIS, percepciones o sellado de tarjeta o banco.', TRUE, '🏦'),
(NULL, 'Finanzas', 'Monotributo / AFIP',           'Monotributo, autónomos, Ganancias o aportes previsionales propios.', TRUE, '🏦'),

-- Otros
(NULL, 'Otros', 'Regalos / donaciones', 'Regalos a terceros y donaciones.', TRUE, '🗂️'),

-- Sistema (reservada — nombres matcheados por el código Go, no cambiar)
(NULL, 'Sistema', 'Saldo inicial',         'Movimiento de apertura de cuenta.', TRUE, '⚙️'),
(NULL, 'Sistema', 'Rendimiento inversión', 'Ganancias o pérdidas de inversiones.', TRUE, '⚙️'),
(NULL, 'Sistema', 'Transferencia',         'Transferencia entre cuentas propias.', TRUE, '⚙️'),

-- Pendiente de revisión (el LLM devuelve esto cuando no puede clasificar)
(NULL, 'PENDING_REVIEW', 'PENDING_REVIEW', 'Categoría y subcategoría a revisión manual.', TRUE, '⏳');

-- +goose Down
-- Removes the newly-seeded global rows and restores the retired ones.
-- The WHERE clause on the restore targets exactly the rows this migration's
-- Up retired (user_id IS NULL + is_global FALSE + deleted_at set is a
-- combination no other code path produces on a previously-global row).
DELETE FROM subcategories WHERE is_global = TRUE;
UPDATE subcategories SET deleted_at = NULL, is_global = TRUE
    WHERE user_id IS NULL AND is_global = FALSE AND deleted_at IS NOT NULL;
