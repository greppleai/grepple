package testdata.kotlin

data class User(val id: String, val name: String)

class UserService(private val users: List<User>) {
    fun findUser(id: String): User? {
        return users.find { it.id == id }
    }

    fun printUser(id: String) {
        val user = findUser(id) ?: return
        println(user.name.trim())
    }
}
