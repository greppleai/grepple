package shop

data class Product(val sku: String, val name: String, val stock: Int)

class Inventory(private val products: MutableList<Product>) {
    fun find(sku: String): Product? =
        products.firstOrNull { it.sku == sku }

    fun restock(sku: String, amount: Int) {
        val index = products.indexOfFirst { it.sku == sku }
        if (index >= 0) {
            val current = products[index]
            products[index] = current.copy(stock = current.stock + amount)
        }
    }

    fun lowStock(threshold: Int): List<Product> =
        products.filter { it.stock < threshold }
}
