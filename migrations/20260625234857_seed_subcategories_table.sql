-- +goose Up
INSERT INTO subcategories (user_id, category, subcategory, description, is_global) VALUES

-- Alimentación
(NULL, 'Alimentación', 'Supermercado',  'Compras grandes o generales en supermercados (Coto, Carrefour, Disco). NO incluye compras específicas como carnicería o verdulería.', TRUE),
(NULL, 'Alimentación', 'Carnicería',    'Compra de carne roja en carnicerías.', TRUE),
(NULL, 'Alimentación', 'Verdulería',    'Compra de frutas y verduras.', TRUE),
(NULL, 'Alimentación', 'Panadería',     'Pan, facturas, medialunas y productos de panadería.', TRUE),
(NULL, 'Alimentación', 'Pollería',      'Compra de pollo u otras aves.', TRUE),
(NULL, 'Alimentación', 'Huevería',      'Compra de huevos.', TRUE),
(NULL, 'Alimentación', 'Pescadería',    'Compra de pescado o mariscos.', TRUE),
(NULL, 'Alimentación', 'Almacén',       'Compras pequeñas en almacenes de barrio, kioscos o minimercados.', TRUE),

-- Vivienda
(NULL, 'Vivienda', 'Alquiler',          'Pago mensual de alquiler de vivienda.', TRUE),
(NULL, 'Vivienda', 'Expensas',          'Expensas de edificio o barrio cerrado.', TRUE),
(NULL, 'Vivienda', 'Agua',              'Factura de agua (AySA u otra empresa).', TRUE),
(NULL, 'Vivienda', 'Gas',               'Factura de gas (Metrogas, Naturgy).', TRUE),
(NULL, 'Vivienda', 'Luz',               'Factura de electricidad (Edesur, Edenor).', TRUE),
(NULL, 'Vivienda', 'Internet',          'Servicio de internet o cable (Fibertel, Telecentro).', TRUE),
(NULL, 'Vivienda', 'Cochera',           'Alquiler mensual de cochera fija.', TRUE),
(NULL, 'Vivienda', 'Impuestos dto.',    'Impuestos de propiedad inmueble (ABL, AGIP, municipales) relacionados a departamento. NO vehículos.', TRUE),
(NULL, 'Vivienda', 'Seguro hogar',      'Seguro de vivienda o contenido del hogar.', TRUE),
(NULL, 'Vivienda', 'Proyecto hogar',    'Pago de impuestos municipales, provinciales, agua, y todo lo relacionado al futuro hogar. Actualmente es un lote / terreno baldío.', TRUE),
(NULL, 'Vivienda', 'Repuestos hogar',   'Reparaciones, mantenimiento y repuestos del hogar.', TRUE),

-- Transporte
(NULL, 'Transporte', 'Combustible auto',     'Nafta o GNC para automóvil.', TRUE),
(NULL, 'Transporte', 'Combustible moto',     'Nafta para motocicleta.', TRUE),
(NULL, 'Transporte', 'Seguro auto',          'Seguro de automóvil.', TRUE),
(NULL, 'Transporte', 'Seguro moto',          'Seguro de motocicleta.', TRUE),
(NULL, 'Transporte', 'Service auto',         'Taller mecánico, service o mantenimiento de auto.', TRUE),
(NULL, 'Transporte', 'Service moto',         'Taller mecánico de moto.', TRUE),
(NULL, 'Transporte', 'Repuestos auto',       'Compra de repuestos o accesorios para auto.', TRUE),
(NULL, 'Transporte', 'Repuestos moto',       'Repuestos o accesorios para moto.', TRUE),
(NULL, 'Transporte', 'Impuesto mun. auto',   'Patente municipal de automóvil.', TRUE),
(NULL, 'Transporte', 'Impuesto prov. auto',  'Patente provincial o ARBA de automóvil.', TRUE),
(NULL, 'Transporte', 'Impuesto mun. moto',   'Patente municipal de moto.', TRUE),
(NULL, 'Transporte', 'Impuesto prov. moto',  'Patente provincial de moto.', TRUE),
(NULL, 'Transporte', 'Estacionamiento',      'Playa de estacionamiento, cochera temporal o parquímetro.', TRUE),
(NULL, 'Transporte', 'Peaje',                'Peajes de autopistas o rutas.', TRUE),
(NULL, 'Transporte', 'Uber / taxi',          'Viajes en Uber, Cabify o taxi.', TRUE),
(NULL, 'Transporte', 'Transporte público',   'SUBE, colectivo, tren o subte.', TRUE),

-- Salud
(NULL, 'Salud', 'Medicamentos',       'Medicamentos con receta (antibióticos, sertralina, etc.).', TRUE),
(NULL, 'Salud', 'Consulta médica',    'Consultas con médicos clínicos o especialistas.', TRUE),
(NULL, 'Salud', 'Obra social / prepaga', 'Cuota mensual de cobertura médica.', TRUE),
(NULL, 'Salud', 'Odontología',        'Dentista y tratamientos dentales.', TRUE),
(NULL, 'Salud', 'Óptica',             'Lentes o consultas oftalmológicas.', TRUE),
(NULL, 'Salud', 'Psicología',         'Sesiones de psicólogo.', TRUE),
(NULL, 'Salud', 'Análisis / estudios','Estudios médicos: laboratorio, radiografías, análisis clínicos.', TRUE),
(NULL, 'Salud', 'Farmacia general',   'Productos sin receta: cremas, vitaminas, higiene personal.', TRUE),

-- Ocio y salidas
(NULL, 'Ocio y salidas', 'Restaurante',      'Salidas a comer en restaurantes.', TRUE),
(NULL, 'Ocio y salidas', 'Café / bar',       'Cafés, bares, cervezas y bebidas alcohólicas.', TRUE),
(NULL, 'Ocio y salidas', 'Heladería',        'Compra de helado.', TRUE),
(NULL, 'Ocio y salidas', 'Delivery',         'Pedidos por apps (PedidosYa, Rappi, etc.).', TRUE),
(NULL, 'Ocio y salidas', 'Cine / teatro',    'Entradas a cine, teatro o espectáculos.', TRUE),
(NULL, 'Ocio y salidas', 'Hobby',            'Actividades recreativas personales (gaming, arte, música, etc.).', TRUE),
(NULL, 'Ocio y salidas', 'Viajes / vacaciones', 'Gastos de viajes: vuelos, hoteles, excursiones.', TRUE),
(NULL, 'Ocio y salidas', 'Deporte externo',  'Alquiler de canchas o clases deportivas externas.', TRUE),

-- Bienestar
(NULL, 'Bienestar', 'Gimnasio',               'Cuota mensual de gimnasio.', TRUE),
(NULL, 'Bienestar', 'Peluquería / barbería',   'Corte de pelo o barbería.', TRUE),
(NULL, 'Bienestar', 'Estética',                'Tratamientos estéticos (depilación, skincare, etc.).', TRUE),
(NULL, 'Bienestar', 'Indumentaria deportiva',  'Ropa deportiva.', TRUE),

-- Indumentaria
(NULL, 'Indumentaria', 'Ropa',        'Ropa en general.', TRUE),
(NULL, 'Indumentaria', 'Calzado',     'Zapatillas o zapatos.', TRUE),
(NULL, 'Indumentaria', 'Accesorios',  'Carteras, cinturones, mochilas u otros accesorios.', TRUE),

-- Tecnología
(NULL, 'Tecnología', 'Celular',       'Compra de teléfono móvil.', TRUE),
(NULL, 'Tecnología', 'Computadora',   'Notebook, PC o laptop.', TRUE),
(NULL, 'Tecnología', 'Periféricos',   'Teclado, mouse, monitor u otros periféricos.', TRUE),
(NULL, 'Tecnología', 'Accesorios tech', 'Cables, cargadores, auriculares.', TRUE),
(NULL, 'Tecnología', 'Reparaciones',  'Reparación de dispositivos electrónicos.', TRUE),

-- Suscripciones
(NULL, 'Suscripciones', 'Streaming (video)',   'Servicios de video (Netflix, Disney+, HBO, YouTube Premium).', TRUE),
(NULL, 'Suscripciones', 'Streaming (música)',  'Servicios de música (Spotify, Apple Music).', TRUE),
(NULL, 'Suscripciones', 'Software',            'Suscripciones a software (Adobe, herramientas digitales).', TRUE),
(NULL, 'Suscripciones', 'Educación online',    'Plataformas educativas (Udemy, Platzi, Coursera).', TRUE),
(NULL, 'Suscripciones', 'Nube / storage',      'Almacenamiento en la nube (iCloud, Google Drive, Dropbox).', TRUE),
(NULL, 'Suscripciones', 'Otros servicios',     'Otros servicios digitales recurrentes.', TRUE),

-- Educación
(NULL, 'Educación', 'Idiomas',             'Clases de idiomas.', TRUE),
(NULL, 'Educación', 'Cursos online',       'Cursos de capacitación profesional.', TRUE),
(NULL, 'Educación', 'Libros',              'Compra de libros físicos o digitales.', TRUE),
(NULL, 'Educación', 'Material educativo',  'Fotocopias, apuntes y materiales de estudio.', TRUE),

-- Inversiones
(NULL, 'Inversiones', 'Jubilación',       'Inversiones a largo plazo (CEDEARs, ahorro previsional).', TRUE),
(NULL, 'Inversiones', 'Cripto',           'Compra o venta de criptomonedas.', TRUE),
(NULL, 'Inversiones', 'Dólares',          'Compra o venta de dólares (físicos o digitales).', TRUE),
(NULL, 'Inversiones', 'FCI',              'Fondos comunes de inversión.', TRUE),
(NULL, 'Inversiones', 'Billetera virtual','Billetera virtual, Mercado Pago, UALÁ, Naranja.', TRUE),
(NULL, 'Inversiones', 'Otros activos',    'Otras inversiones no clasificadas.', TRUE),

-- Otros
(NULL, 'Otros', 'Regalos / Donaciones', 'Gasto destinado a regalos y/o donaciones.', TRUE),

-- Finanzas
(NULL, 'Finanzas', 'Cargos bancarios',  'Mantenimiento de cuenta bancaria u otros cargos.', TRUE),
(NULL, 'Finanzas', 'Impuestos tarjeta', 'IVA, impuesto PAIS, percepciones de tarjeta.', TRUE),
(NULL, 'Finanzas', 'Comisiones',        'Comisiones financieras, bancarias o de brokers.', TRUE),

-- Ingresos
(NULL, 'Ingresos', 'Sueldo',           'Salario mensual en relación de dependencia.', TRUE),
(NULL, 'Ingresos', 'Freelance',        'Ingresos por trabajos independientes.', TRUE),
(NULL, 'Ingresos', 'Inversiones',      'Rendimientos, dividendos o intereses.', TRUE),
(NULL, 'Ingresos', 'Reintegros',       'Devoluciones o reintegros de dinero.', TRUE),
(NULL, 'Ingresos', 'Otros ingresos',   'Otros ingresos no clasificados.', TRUE),

-- Pendiente de revisión (el LLM devuelve esto cuando no puede clasificar)
(NULL, 'PENDING_REVIEW', 'PENDING_REVIEW', 'Categoría y subcategoría a revisión manual.', TRUE),

-- Asignadas por el sistema
(NULL, 'Sistema', 'Saldo inicial', 'Movimiento de apertura de cuenta', TRUE),
(NULL, 'Sistema', 'Rendimiento inversión', 'Ganancias o pérdidas de inversiones', TRUE);

-- +goose Down
DELETE FROM subcategories WHERE is_global = TRUE;