import 'package:sqflite/sqflite.dart';
import 'package:path/path.dart' as p;

class LocalDatabase {
  static const _dbName = 'moon_eye.db';
  static const _dbVersion = 1;

  static final LocalDatabase instance = LocalDatabase._internal();

  Database? _db;

  LocalDatabase._internal();

  Future<Database> get database async {
    if (_db != null) return _db!;
    _db = await _init();
    return _db!;
  }

  Future<Database> _init() async {
    final dbPath = await getDatabasesPath();
    final path = p.join(dbPath, _dbName);

    return openDatabase(
      path,
      version: _dbVersion,
      onCreate: _onCreate,
    );
  }

  Future<void> _onCreate(Database db, int version) async {
    await db.execute('''
      CREATE TABLE transactions (
        id TEXT PRIMARY KEY,
        user_id TEXT NOT NULL,
        account_id TEXT NOT NULL,
        amount REAL NOT NULL,
        currency TEXT NOT NULL,
        type TEXT NOT NULL,
        category_id TEXT,
        description TEXT,
        occurred_at TEXT NOT NULL,
        metadata TEXT,
        version INTEGER NOT NULL,
        last_modified TEXT NOT NULL,
        source TEXT NOT NULL,
        sheets_row_id TEXT,
        deleted INTEGER NOT NULL DEFAULT 0
      );
    ''');

    await db.execute('''
      CREATE TABLE pending_ops (
        id INTEGER PRIMARY KEY AUTOINCREMENT,
        op_type TEXT NOT NULL,
        entity_type TEXT NOT NULL,
        entity_id TEXT NOT NULL,
        payload TEXT NOT NULL,
        created_at TEXT NOT NULL,
        idempotency_key TEXT NOT NULL
      );
    ''');

    await db.execute('''
      CREATE TABLE devices (
        id TEXT PRIMARY KEY,
        user_id TEXT NOT NULL,
        last_sync_at TEXT,
        created_at TEXT NOT NULL
      );
    ''');
  }

  Future<void> upsertTransaction(Map<String, dynamic> tx) async {
    final db = await database;
    await db.insert(
      'transactions',
      tx,
      conflictAlgorithm: ConflictAlgorithm.replace,
    );
  }

  Future<int> enqueueOp(Map<String, dynamic> op) async {
    final db = await database;
    return db.insert('pending_ops', op);
  }

  Future<List<Map<String, dynamic>>> readPendingOps({int limit = 100}) async {
    final db = await database;
    return db.query(
      'pending_ops',
      orderBy: 'id ASC',
      limit: limit,
    );
  }
}

/// Example usage:
///
/// final db = LocalDatabase.instance;
/// await db.upsertTransaction({...});
/// await db.enqueueOp({...});
/// final ops = await db.readPendingOps();

